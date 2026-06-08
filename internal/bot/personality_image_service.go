package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateImage реализует генерацию изображения на основе личности
func (p *PersonalityBasedImageService) GenerateImage(ctx context.Context, chatID int64) (*GeneratedImage, error) {
	log.Printf("[PersonalityImageGen] Начинаем генерацию для чата %d", chatID)

	// 1. Собираем данные личности
	personalityData, err := p.gatherPersonalityData(chatID)
	if err != nil {
		return nil, fmt.Errorf("ошибка сбора данных личности: %w", err)
	}

	// 2. Первый этап: анализ личности и создание контекста
	analysisPrompt := p.buildPersonalityAnalysisPrompt(personalityData)

	log.Printf("[PersonalityImageGen] Запускаем анализ личности...")
	analysisResponse, err := p.bot.llm.GenerateArbitraryResponse(analysisPrompt, "", 0.7)
	if err != nil {
		return nil, fmt.Errorf("ошибка анализа личности: %w", err)
	}

	// 3. Второй этап: генерация изображения
	log.Printf("[PersonalityImageGen] Анализ завершен, генерируем изображение...")

	// Получаем случайное базовое изображение
	baseImage, err := p.getRandomLunaAppearanceImage()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения базового изображения: %w", err)
	}

	// Формируем финальный промпт для генерации
	imagePrompt := p.buildImageGenerationPrompt(analysisResponse, personalityData)

	// Генерируем описание для изображения с помощью обычного LLM
	imageDescription, err := p.bot.llm.GenerateContentWithImage(ctx, imagePrompt, baseImage, "Создай описание для генерации изображения")
	if err != nil {
		return nil, fmt.Errorf("ошибка создания описания изображения: %w", err)
	}

	// Генерируем изображение используя интерфейс LLMClient
	generatedImageData, err := p.bot.llm.GenerateImageWithEdit(ctx, baseImage, imageDescription)
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации изображения: %w", err)
	}

	// 4. Создаем результат
	result := &GeneratedImage{
		ImageData:   generatedImageData,
		Caption:     p.generateCaption(analysisResponse),
		ServiceName: p.GetServiceName(),
		ChatID:      chatID,
		Timestamp:   time.Now(),
	}

	log.Printf("[PersonalityImageGen] ✅ Изображение успешно сгенерировано для чата %d", chatID)
	return result, nil
}

// GetServiceName возвращает имя сервиса
func (p *PersonalityBasedImageService) GetServiceName() string {
	return "personality_based"
}

// IsEnabled проверяет, включен ли сервис
func (p *PersonalityBasedImageService) IsEnabled() bool {
	return p.enabled && p.bot.llm != nil
}

// gatherPersonalityData собирает данные личности из всех источников
func (p *PersonalityBasedImageService) gatherPersonalityData(chatID int64) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Базовая информация о чате
	data["chat_id"] = chatID
	data["timestamp"] = time.Now().Unix()

	// 1. Получаем существующую PersonalityMemory из хранилища
	personalityMemory, err := p.bot.storage.GetPersonalityMemory(chatID)
	if err == nil && personalityMemory != nil {
		data["personality_memory"] = personalityMemory
		data["recent_topics"] = personalityMemory.RecentTopics
		data["static_personality"] = personalityMemory.StaticPersonality
		log.Printf("[PersonalityImageGen] Собрана PersonalityMemory: тем=%d, статическая личность есть=%v",
			len(personalityMemory.RecentTopics), personalityMemory.StaticPersonality != "")
	}

	// 2. Настройки чата (упрощенная версия)
	data["chat_context"] = map[string]interface{}{
		"chat_id": chatID,
		"active":  true,
	}

	// 3. Получаем недавние сообщения для контекста с учетом IMAGE_GENERATION_CONTEXT_WINDOW
	contextWindow := p.bot.config.ImageGenerationContextWindow
	if contextWindow <= 0 {
		contextWindow = 50 // fallback значение
	}
	log.Printf("[PersonalityImageGen] Используем контекстное окно для генерации изображений: %d сообщений", contextWindow)
	recentMessages, err := p.bot.storage.GetMessages(chatID, contextWindow)
	if err == nil && len(recentMessages) > 0 {
		// Упрощенная версия сообщений для контекста
		var messageContexts []map[string]interface{}
		for _, msg := range recentMessages {
			messageContexts = append(messageContexts, map[string]interface{}{
				"user_name":  msg.From.UserName,
				"text":       msg.Text,
				"timestamp":  time.Unix(int64(msg.Date), 0),
				"message_id": msg.MessageID,
			})
		}
		data["recent_messages"] = messageContexts
		log.Printf("[PersonalityImageGen] Собрано недавних сообщений: %d", len(messageContexts))
	}

	// 4. Добавляем информацию о пользователях через UserValidator (если доступен)
	if p.bot.userValidator != nil {
		data["user_disambiguation_enabled"] = true
		log.Printf("[PersonalityImageGen] UserValidator доступен")
	} else {
		data["user_disambiguation_enabled"] = false
		log.Printf("[PersonalityImageGen] UserValidator недоступен")
	}

	// 5. Статические данные личности (можно вынести в конфигурацию)
	data["static_personality"] = map[string]interface{}{
		"core_traits":         []string{"analytical", "sarcastic", "creative", "spontaneous"},
		"humor_style":         "sarcastic",
		"communication_style": "direct",
		"creativity_level":    "high",
		"analytical_tendency": "high",
	}

	// 6. Временная информация для контекста
	now := time.Now()
	data["temporal_context"] = map[string]interface{}{
		"hour":        now.Hour(),
		"day_of_week": now.Weekday().String(),
		"is_weekend":  now.Weekday() == time.Saturday || now.Weekday() == time.Sunday,
		"is_daytime":  now.Hour() >= 8 && now.Hour() <= 22,
	}

	log.Printf("[PersonalityImageGen] ✅ Данные личности собраны для чата %d", chatID)
	return data, nil
}

// buildPersonalityAnalysisPrompt создает промпт для первого этапа анализа личности
func (p *PersonalityBasedImageService) buildPersonalityAnalysisPrompt(personalityData map[string]interface{}) string {
	basePrompt := p.prePrompt
	if basePrompt == "" {
		basePrompt = `Проанализируй данные личности и создай психологический портрет для генерации селфи. 

Учти следующие аспекты:
1. Текущее эмоциональное состояние и настроение
2. Доминирующие черты характера и убеждения
3. Социальный контекст и отношения
4. Когнитивный стиль и способ мышления
5. Недавние события и воспоминания

Ответь в формате JSON с полями:
- mood: текущее настроение (playful/serious/sarcastic/melancholic/etc)
- dominant_traits: массив основных черт характера
- social_energy: уровень социальной энергии (high/medium/low)
- creative_state: творческое состояние (inspired/routine/blocked)
- background_context: описание подходящего заднего плана для фото
- outfit_style: стиль одежды (casual/elegant/rebellious/cozy/etc)
- pose_suggestion: предложение позы (natural/confident/playful/contemplative/etc)

Данные личности:`
	}

	// Сериализуем данные личности в JSON для промпта
	personalityJSON, err := json.MarshalIndent(personalityData, "", "  ")
	if err != nil {
		log.Printf("[PersonalityImageGen] Ошибка сериализации данных: %v", err)
		personalityJSON = []byte(`{"error": "failed to serialize personality data"}`)
	}

	return fmt.Sprintf("%s\n\n%s", basePrompt, string(personalityJSON))
}

// buildImageGenerationPrompt создает промпт для генерации изображения
func (p *PersonalityBasedImageService) buildImageGenerationPrompt(analysisResponse string, personalityData map[string]interface{}) string {
	baseImagePrompt := p.imageGenPrompt
	if baseImagePrompt == "" {
		baseImagePrompt = `Создай реалистичное селфи на основе анализа личности. Используй следующий контекст:`
	}

	// Включаем анализ личности в промпт
	prompt := fmt.Sprintf("Анализ личности:\n%s\n\n%s", analysisResponse, baseImagePrompt)

	// Заменяем плейсхолдер фона, если он есть в промпте
	if strings.Contains(prompt, "{BACKGROUND_DESCRIPTION}") {
		backgroundDesc := p.extractBackgroundFromAnalysis(analysisResponse)
		prompt = strings.ReplaceAll(prompt, "{BACKGROUND_DESCRIPTION}", backgroundDesc)
	}

	return prompt
}

// extractBackgroundFromAnalysis извлекает описание фона из анализа
func (p *PersonalityBasedImageService) extractBackgroundFromAnalysis(analysis string) string {
	// Пытаемся извлечь background_context из JSON ответа
	var analysisData map[string]interface{}
	if err := json.Unmarshal([]byte(analysis), &analysisData); err == nil {
		if bg, ok := analysisData["background_context"].(string); ok {
			return bg
		}
	}

	// Фолбэк описания
	return "домашняя обстановка с мягким естественным освещением"
}

// generateCaption создает подпись к изображению
func (p *PersonalityBasedImageService) generateCaption(analysisResponse string) string {
	// Можно создать умную подпись на основе анализа
	// Пока простая версия
	return "" // Оставляем пустой, чтобы не засорять чат
}

// getRandomLunaAppearanceImage возвращает случайное изображение из папки luna_appearance
func (p *PersonalityBasedImageService) getRandomLunaAppearanceImage() ([]byte, error) {
	return getRandomFileFromDir(p.lunaAppearanceDir, []string{".jpg", ".jpeg", ".png"})
}

// getRandomFileFromDir возвращает случайный файл из директории с указанными расширениями
func getRandomFileFromDir(dirPath string, extensions []string) ([]byte, error) {
	log.Printf("[ImageGeneration] 🔍 Ищем файлы в директории: %s", dirPath)

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения папки %s (возможно, директория не существует или нет прав доступа): %w", dirPath, err)
	}

	log.Printf("[ImageGeneration] 📂 Найдено файлов в директории: %d", len(files))

	// Фильтруем только файлы с нужными расширениями
	var validFiles []string
	for _, file := range files {
		if !file.IsDir() {
			ext := strings.ToLower(filepath.Ext(file.Name()))
			for _, validExt := range extensions {
				if ext == validExt {
					validFiles = append(validFiles, file.Name())
					break
				}
			}
		}
	}

	if len(validFiles) == 0 {
		return nil, fmt.Errorf("в папке %s не найдено файлов с расширениями %v", dirPath, extensions)
	}

	// Выбираем случайный файл
	selectedFile := validFiles[rand.Intn(len(validFiles))]
	filePath := filepath.Join(dirPath, selectedFile)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла %s: %w", filePath, err)
	}

	log.Printf("[ImageGeneration] Выбрано случайное изображение: %s", selectedFile)
	return data, nil
}
