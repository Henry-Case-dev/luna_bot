package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ImageGenerationService - основной сервис генерации изображений
// TODO(cln-07): Мигрировать на ImageGenerator capability-интерфейс из llm пакета.
// Сейчас сервис работает через PersonalityBasedImageService, который использует
// прямой вызов LLM-клиента вместо ImageGenerator.GenerateImageWithEdit().
// После миграции:
//   - ImageGenerationService должен принимать llm.ImageGenerator из ProviderRegistry
//   - PersonalityBasedImageService должен быть заменён на прямой вызов ImageGenerator
//   - Метод DecisionMechanismShouldGenerate должен использовать capability-based роутинг
type ImageGenerationService struct {
	bot                     *Bot
	enabled                 bool
	subServices             map[string]ImageSubService
	frequencyHours          int // Частота отправки в часах
	lastGenerationTimes     map[int64]time.Time
	maxGenerationsPerPeriod int
	generationCount         map[int64]int
	mu                      sync.RWMutex
	lunaAppearanceDir       string
}

// ImageSubService - интерфейс для подсервисов генерации изображений
type ImageSubService interface {
	GenerateImage(ctx context.Context, chatID int64) (*GeneratedImage, error)
	GetServiceName() string
	IsEnabled() bool
}

// GeneratedImage - результат генерации изображения
type GeneratedImage struct {
	ImageData   []byte
	Caption     string
	ServiceName string
	ChatID      int64
	Timestamp   time.Time
}

// PersonalityBasedImageService - подсервис генерации на основе личности
type PersonalityBasedImageService struct {
	bot                   *Bot
	enabled               bool
	prePrompt             string
	imageGenPrompt        string
	additionalInstruction string
	lunaAppearanceDir     string
}

// NewImageGenerationService создает новый сервис генерации изображений
func NewImageGenerationService(bot *Bot, cfg *config.Config) *ImageGenerationService {
	log.Printf("[ImageGeneration] Инициализация сервиса генерации изображений")

	// Определяем путь к директории luna_appearance
	workDir, err := os.Getwd()
	if err != nil {
		log.Printf("[ImageGeneration] ⚠️ Не удалось получить рабочую директорию: %v", err)
		workDir = "."
	}

	lunaAppearanceDir := filepath.Join(workDir, "luna_appearance")

	// Проверяем существование директории
	if _, err := os.Stat(lunaAppearanceDir); os.IsNotExist(err) {
		log.Printf("[ImageGeneration] ⚠️ Директория %s не найдена, пробуем относительный путь", lunaAppearanceDir)
		lunaAppearanceDir = "luna_appearance"

		// Дополнительная проверка
		if _, err := os.Stat(lunaAppearanceDir); os.IsNotExist(err) {
			log.Printf("[ImageGeneration] ❌ Директория luna_appearance не найдена ни по абсолютному, ни по относительному пути")
		}
	}

	log.Printf("[ImageGeneration] 📁 Используем директорию изображений: %s", lunaAppearanceDir)

	service := &ImageGenerationService{
		bot:                     bot,
		enabled:                 true, // Пока захардкодим, потом вынесем в env
		subServices:             make(map[string]ImageSubService),
		frequencyHours:          cfg.ImageGenFrequencyHours,
		lastGenerationTimes:     make(map[int64]time.Time),
		maxGenerationsPerPeriod: 2,
		generationCount:         make(map[int64]int),
		lunaAppearanceDir:       lunaAppearanceDir,
	}

	// Инициализируем подсервис генерации на основе личности
	personalityService := &PersonalityBasedImageService{
		bot:                   bot,
		enabled:               true,
		prePrompt:             cfg.ImageGenPrePrompt,
		imageGenPrompt:        cfg.FreeWillImageGenPrompt,
		additionalInstruction: "", // Используем только imageGenPrompt из конфигурации
		lunaAppearanceDir:     lunaAppearanceDir,
	}

	// Регистрируем подсервис
	service.RegisterSubService("personality_based", personalityService)

	log.Printf("[ImageGeneration] ✅ Сервис инициализирован. Частота: %d часов, подсервисов: %d",
		service.frequencyHours, len(service.subServices))

	return service
}

// RegisterSubService регистрирует новый подсервис
func (s *ImageGenerationService) RegisterSubService(name string, subService ImageSubService) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subServices[name] = subService
	log.Printf("[ImageGeneration] Зарегистрирован подсервис: %s", name)
}

// ShouldGenerateImage проверяет, нужно ли генерировать изображение для чата
func (s *ImageGenerationService) ShouldGenerateImage(chatID int64) bool {
	if !s.enabled {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	// Проверяем время последней генерации
	if lastTime, exists := s.lastGenerationTimes[chatID]; exists {
		if now.Sub(lastTime) < time.Duration(s.frequencyHours)*time.Hour {
			return false
		}
	}

	// Проверяем лимит генераций за период
	count := s.generationCount[chatID]
	return count < s.maxGenerationsPerPeriod
}

// GenerateImageForChat генерирует изображение для указанного чата
func (s *ImageGenerationService) GenerateImageForChat(ctx context.Context, chatID int64, serviceName string) (*GeneratedImage, error) {
	if !s.enabled {
		return nil, fmt.Errorf("сервис генерации изображений отключен")
	}

	s.mu.RLock()
	subService, exists := s.subServices[serviceName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("подсервис %s не найден", serviceName)
	}

	if !subService.IsEnabled() {
		return nil, fmt.Errorf("подсервис %s отключен", serviceName)
	}

	log.Printf("[ImageGeneration] Начинаем генерацию изображения для чата %d с помощью %s", chatID, serviceName)

	image, err := subService.GenerateImage(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации изображения: %w", err)
	}

	// Обновляем статистику
	s.mu.Lock()
	s.lastGenerationTimes[chatID] = time.Now()
	s.generationCount[chatID]++
	s.mu.Unlock()

	log.Printf("[ImageGeneration] ✅ Изображение успешно сгенерировано для чата %d", chatID)
	return image, nil
}

// SendGeneratedImage отправляет сгенерированное изображение в чат
func (s *ImageGenerationService) SendGeneratedImage(image *GeneratedImage) error {
	if image == nil {
		return fmt.Errorf("изображение не может быть nil")
	}

	// Создаем фото для отправки
	photoConfig := tgbotapi.NewPhoto(image.ChatID, tgbotapi.FileBytes{
		Name:  fmt.Sprintf("generated_%s_%d.jpg", image.ServiceName, image.Timestamp.Unix()),
		Bytes: image.ImageData,
	})

	if image.Caption != "" {
		photoConfig.Caption = image.Caption
	}

	// Отправляем через бота
	_, err := s.bot.api.Send(photoConfig)
	if err != nil {
		return fmt.Errorf("ошибка отправки изображения: %w", err)
	}

	log.Printf("[ImageGeneration] ✅ Изображение отправлено в чат %d", image.ChatID)
	return nil
}

// DecisionMechanismShouldGenerate - механизм принятия решения для Free Will
func (s *ImageGenerationService) DecisionMechanismShouldGenerate(chatID int64, contextData map[string]interface{}) bool {
	if !s.enabled {
		return false
	}

	// Базовая проверка времени
	if !s.ShouldGenerateImage(chatID) {
		return false
	}

	// Здесь можно добавить более сложную логику принятия решений
	// на основе контекста, настроения, активности в чате и т.д.

	// Простая вероятностная модель (можно расширить)
	probability := 0.3 // 30% базовая вероятность

	// Можно учесть настроение
	if mood, ok := contextData["mood"].(string); ok {
		switch mood {
		case "playful", "sarcastic":
			probability += 0.2
		case "serious":
			probability -= 0.1
		}
	}

	// Можно учесть время суток
	hour := time.Now().Hour()
	if hour >= 8 && hour <= 22 { // Дневное время
		probability += 0.1
	}

	return rand.Float64() < probability
}

// GetRandomLunaAppearanceImage возвращает случайное изображение из папки luna_appearance
func (s *ImageGenerationService) GetRandomLunaAppearanceImage() ([]byte, error) {
	files, err := os.ReadDir(s.lunaAppearanceDir)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения папки %s: %w", s.lunaAppearanceDir, err)
	}

	// Фильтруем только изображения
	var imageFiles []string
	for _, file := range files {
		if !file.IsDir() {
			ext := strings.ToLower(filepath.Ext(file.Name()))
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
				imageFiles = append(imageFiles, file.Name())
			}
		}
	}

	if len(imageFiles) == 0 {
		return nil, fmt.Errorf("в папке %s не найдено изображений", s.lunaAppearanceDir)
	}

	// Выбираем случайное изображение
	selectedFile := imageFiles[rand.Intn(len(imageFiles))]
	filePath := filepath.Join(s.lunaAppearanceDir, selectedFile)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла %s: %w", filePath, err)
	}

	log.Printf("[ImageGeneration] Выбрано случайное изображение: %s", selectedFile)
	return data, nil
}

// IsEnabled возвращает статус включенности сервиса
func (s *ImageGenerationService) IsEnabled() bool {
	return s.enabled
}

// GetSubServices возвращает список зарегистрированных подсервисов
func (s *ImageGenerationService) GetSubServices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var services []string
	for name := range s.subServices {
		services = append(services, name)
	}
	return services
}
