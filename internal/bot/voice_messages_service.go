package bot

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/elevenlabs"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// VoiceMessageService управляет отправкой голосовых сообщений
type VoiceMessageService struct {
	bot             *Bot
	elevenLabs      *elevenlabs.Client
	enabled         bool
	intervalMin     int
	intervalMax     int
	tempDir         string
	mutex           sync.RWMutex
	messageCounters map[int64]int // Счетчики сообщений по чатам
	nextTargets     map[int64]int // Следующие цели для каждого чата
	randSource      *rand.Rand
}

// NewVoiceMessageService создает новый сервис голосовых сообщений
func NewVoiceMessageService(bot *Bot) (*VoiceMessageService, error) {
	// Проверяем наличие API ключа
	if bot.config.ElevenLabsAPIKey == "" {
		log.Printf("[VoiceMessages] ElevenLabs API ключ не установлен - сервис отключен")
		return &VoiceMessageService{
			bot:     bot,
			enabled: false,
		}, nil
	}

	// Создаем клиент ElevenLabs
	plan := elevenlabs.ElevenLabsPlan(bot.config.ElevenLabsPlan)
	voiceConfig := elevenlabs.VoiceConfig{
		Stability:       bot.config.ElevenLabsStability,
		SimilarityBoost: bot.config.ElevenLabsSimilarityBoost,
		Style:           bot.config.ElevenLabsStyle,
		UseSpeakerBoost: bot.config.ElevenLabsUseSpeakerBoost,
		Speed:           bot.config.ElevenLabsSpeed,
		StylePrompt:     bot.config.ElevenLabsStylePrompt,
		EmotionPrompt:   bot.config.ElevenLabsEmotionPrompt,
		PacePrompt:      bot.config.ElevenLabsPacePrompt,
		RandomVoice:     bot.config.ElevenLabsRandomVoice,
	}

	elevenLabsClient := elevenlabs.NewClientWithVoiceConfig(
		bot.config.ElevenLabsAPIKey,
		bot.config.ElevenLabsVoiceID,
		bot.config.ElevenLabsModel,
		plan,
		bot.config.Debug,
		voiceConfig,
	)

	// Создаем временную директорию если её нет
	tempDir := bot.config.VoiceMessageTempDir
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("[VoiceMessages] Ошибка создания временной директории %s: %v", tempDir, err)
		tempDir = "/tmp" // Fallback
	}

	// Проверяем наличие ffmpeg
	if err := checkFFmpegAvailable(); err != nil {
		return nil, fmt.Errorf("ffmpeg недоступен для конвертации аудио: %w", err)
	}

	source := rand.NewSource(time.Now().UnixNano())
	randGen := rand.New(source)

	service := &VoiceMessageService{
		bot:             bot,
		elevenLabs:      elevenLabsClient,
		enabled:         true,
		intervalMin:     bot.config.MinVoiceMessages,
		intervalMax:     bot.config.MaxVoiceMessages,
		tempDir:         tempDir,
		messageCounters: make(map[int64]int),
		nextTargets:     make(map[int64]int),
		randSource:      randGen,
	}

	log.Printf("[VoiceMessages] Сервис инициализирован: план %s, интервал %d-%d сообщений",
		bot.config.ElevenLabsPlan, bot.config.MinVoiceMessages, bot.config.MaxVoiceMessages)

	return service, nil
}

// checkFFmpegAvailable проверяет доступность ffmpeg
func checkFFmpegAvailable() error {
	cmd := exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg не найден или недоступен: %w", err)
	}
	return nil
}

// OnMessage вызывается при получении нового сообщения в чате
func (vms *VoiceMessageService) OnMessage(chatID int64) {
	// Проверяем, включены ли старые интервальные голосовые сообщения
	if !vms.bot.config.VoiceMessagesEnabled {
		return
	}
	if !vms.enabled {
		return
	}

	vms.mutex.Lock()
	defer vms.mutex.Unlock()

	// Увеличиваем счетчик сообщений для чата
	vms.messageCounters[chatID]++

	// Проверяем, нужно ли инициализировать цель для чата
	if _, exists := vms.nextTargets[chatID]; !exists {
		vms.nextTargets[chatID] = vms.generateNextTarget()
		if vms.bot.config.Debug {
			log.Printf("[DEBUG][VoiceMessages] Chat %d: установлена цель %d сообщений",
				chatID, vms.nextTargets[chatID])
		}
	}

	// Проверяем, достигли ли цели
	if vms.messageCounters[chatID] >= vms.nextTargets[chatID] {
		if vms.bot.config.Debug {
			log.Printf("[DEBUG][VoiceMessages] Chat %d: достигнута цель %d/%d - генерирую голосовое сообщение",
				chatID, vms.messageCounters[chatID], vms.nextTargets[chatID])
		}

		// Сбрасываем счетчик и генерируем новую цель
		vms.messageCounters[chatID] = 0
		vms.nextTargets[chatID] = vms.generateNextTarget()

		// Запускаем генерацию голосового сообщения в горутине
		go vms.generateAndSendVoiceMessage(chatID)
	}
}

// generateNextTarget генерирует следующую цель количества сообщений
func (vms *VoiceMessageService) generateNextTarget() int {
	return vms.intervalMin + vms.randSource.Intn(vms.intervalMax-vms.intervalMin+1)
}

// ForceVoiceMessage принудительно отправляет голосовое сообщение (для админ команды)
func (vms *VoiceMessageService) ForceVoiceMessage(chatID int64) error {
	if !vms.enabled {
		return fmt.Errorf("сервис голосовых сообщений отключен")
	}

	log.Printf("[VoiceMessages] Принудительная генерация голосового сообщения для чата %d", chatID)
	go vms.generateAndSendVoiceMessage(chatID)
	return nil
}

// generateAndSendVoiceMessage генерирует и отправляет голосовое сообщение
func (vms *VoiceMessageService) generateAndSendVoiceMessage(chatID int64) {
	startTime := time.Now()

	// Проверяем дневной лимит
	if !vms.elevenLabs.CanSendVoiceMessage() {
		usage, limit, plan := vms.elevenLabs.GetUsageInfo()
		log.Printf("[VoiceMessages] Chat %d: превышен дневной лимит %d/%d для плана %s",
			chatID, usage, limit, plan)
		vms.bot.sendReply(chatID, "Достигнут дневной лимит голосовых сообщений. Попробуйте завтра!")
		return
	}

	// Генерируем текст сообщения
	text, err := vms.generateVoiceText(chatID)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка генерации текста: %v", chatID, err)
		return
	}

	if vms.bot.config.Debug {
		log.Printf("[DEBUG][VoiceMessages] Chat %d: сгенерирован текст (%d символов): %s",
			chatID, len(text), text)
	}

	// Генерируем аудио через ElevenLabs
	audioData, err := vms.elevenLabs.GenerateAudio(text)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка генерации аудио: %v", chatID, err)
		return
	}

	// Конвертируем MP3 в OGG/Opus для Telegram
	voiceData, err := vms.convertToVoiceFormat(audioData)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка конвертации аудио: %v", chatID, err)
		return
	}

	// Отправляем голосовое сообщение
	if err := vms.sendVoiceMessage(chatID, voiceData); err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка отправки голосового сообщения: %v", chatID, err)
		return
	}

	duration := time.Since(startTime)
	usage, limit, plan := vms.elevenLabs.GetUsageInfo()
	log.Printf("[VoiceMessages] Chat %d: голосовое сообщение отправлено за %s. Использовано: %d/%d (%s)",
		chatID, duration.Round(time.Millisecond), usage, limit, plan)
}

// generateVoiceText генерирует текст для голосового сообщения
func (vms *VoiceMessageService) generateVoiceText(chatID int64) (string, error) {
	// Получаем последние сообщения для контекста как в sendAIResponse
	messagesForContext, err := vms.bot.storage.GetMessages(chatID, vms.bot.config.MaxMessages)
	if err != nil {
		return "", fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	// Формируем контекст так же как в sendAIResponse
	contextMessages := messagesForContext
	if len(contextMessages) > vms.bot.config.ContextWindow {
		contextMessages = contextMessages[len(contextMessages)-vms.bot.config.ContextWindow:]
	}

	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(vms.bot.storage, vms.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(vms.bot.config.DisableUserProfiles)
	contextText := formatter.FormatMessagesXML(chatID, contextMessages)

	log.Printf("[VoiceMessages] Chat %d: Использован унифицированный форматтер для %d сообщений", chatID, len(contextMessages))

	// Генерируем первичный текст
	prompt := vms.bot.config.VoiceMessagesPrompt
	if prompt == "" {
		prompt = "Сгенерируй короткое (1-2 предложения) сообщение для голосового сообщения"
	}

	// Встраиваем личность в промпт голосового сообщения
	enrichedPrompt := vms.bot.enrichPromptWithPersonality(prompt, chatID, "voice")

	response, err := vms.bot.llm.GenerateResponseByType(llm.ResponseTypeVoiceFormat,
		enrichedPrompt,
		contextText,
		float32(vms.bot.config.GeminiTemperatureNormal),
	)
	if err != nil {
		return "", fmt.Errorf("ошибка первичной генерации: %w", err)
	}

	// Очищаем и оптимизируем для голоса
	log.Printf("🧹 [VOICE] Очистка ответа для голосового сообщения в чате %d (исходная длина: %d)", chatID, len(response))
	cleanedText := cleanupLLMResponse(response)
	voiceOptimizedText := vms.optimizeTextForVoice(cleanedText)

	return voiceOptimizedText, nil
}

// optimizeTextForVoice оптимизирует текст для голосового воспроизведения
func (vms *VoiceMessageService) optimizeTextForVoice(text string) string {
	// Удаляем специальные символы и markdown
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "#", "")

	// Заменяем некоторые интернет-сокращения на полные слова
	text = strings.ReplaceAll(text, "нахуй", "на хуй")
	text = strings.ReplaceAll(text, "блядь", "блять")

	// Ограничиваем длину (ElevenLabs имеет лимиты)
	if len(text) > 500 {
		text = text[:497] + "..."
	}

	return text
}

// convertToVoiceFormat конвертирует MP3 в OGG/Opus для Telegram
func (vms *VoiceMessageService) convertToVoiceFormat(mp3Data []byte) ([]byte, error) {
	// Создаем временные файлы
	inputFile := filepath.Join(vms.tempDir, fmt.Sprintf("voice_input_%d.mp3", time.Now().UnixNano()))
	outputFile := filepath.Join(vms.tempDir, fmt.Sprintf("voice_output_%d.ogg", time.Now().UnixNano()))

	// Очистка временных файлов в конце
	defer func() {
		os.Remove(inputFile)
		os.Remove(outputFile)
	}()

	// Записываем MP3 во временный файл
	if err := os.WriteFile(inputFile, mp3Data, 0644); err != nil {
		return nil, fmt.Errorf("ошибка записи входного файла: %w", err)
	}

	// Конвертируем в OGG/Opus используя ffmpeg
	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-ar", "48000",
		"-ac", "1", // Моно
		"-y", // Перезаписать выходной файл
		outputFile,
	)

	if vms.bot.config.Debug {
		log.Printf("[DEBUG][VoiceMessages] FFmpeg команда: %s", cmd.String())
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ошибка конвертации ffmpeg: %w", err)
	}

	// Читаем конвертированный файл
	oggData, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения выходного файла: %w", err)
	}

	return oggData, nil
}

// sendVoiceMessage отправляет голосовое сообщение в Telegram
func (vms *VoiceMessageService) sendVoiceMessage(chatID int64, voiceData []byte) error {
	// Создаем FileBytes для отправки
	voiceMsg := tgbotapi.NewVoice(chatID, tgbotapi.FileBytes{
		Name:  "voice_message.ogg",
		Bytes: voiceData,
	})

	// Отправляем через Telegram API
	sentMsg, err := vms.bot.api.Send(voiceMsg)
	if err != nil {
		return fmt.Errorf("ошибка отправки в Telegram: %w", err)
	}

	// Сохраняем сообщение в истории
	if vms.bot.storage != nil {
		vms.bot.storage.AddMessage(chatID, &sentMsg)
		if vms.bot.config.Debug {
			log.Printf("[DEBUG][VoiceMessages] Голосовое сообщение сохранено в историю")
		}
	}

	// Запускаем транскрибацию голосового сообщения бота в отдельной горутине
	// поскольку Telegram не отправляет боту его собственные сообщения через Updates
	go func() {
		if vms.bot.config.Debug {
			log.Printf("[DEBUG][VoiceMessages] Chat %d: Запуск транскрибации голосового сообщения бота ID %d", chatID, sentMsg.MessageID)
		}

		// Вызываем handleVoiceMessage для транскрибации собственного голосового сообщения
		vms.bot.handleVoiceMessage(&sentMsg)
	}()

	return nil
}

// GetStatus возвращает статус сервиса
func (vms *VoiceMessageService) GetStatus() map[string]interface{} {
	if !vms.enabled {
		return map[string]interface{}{
			"enabled": false,
			"reason":  "ElevenLabs API ключ не установлен",
		}
	}

	vms.mutex.RLock()
	defer vms.mutex.RUnlock()

	usage, limit, plan := vms.elevenLabs.GetUsageInfo()
	remaining := vms.elevenLabs.GetRemainingCredits()

	return map[string]interface{}{
		"enabled":      true,
		"plan":         plan,
		"daily_usage":  usage,
		"daily_limit":  limit,
		"remaining":    remaining,
		"interval_min": vms.intervalMin,
		"interval_max": vms.intervalMax,
		"active_chats": len(vms.messageCounters),
	}
}

// generateAndSendVoiceMessageWithText с заданным текстом для Free Will
func (vms *VoiceMessageService) generateAndSendVoiceMessageWithText(chatID int64, text string) {
	if !vms.enabled {
		log.Printf("[VoiceMessages] Chat %d: сервис отключен", chatID)
		return
	}

	startTime := time.Now()
	log.Printf("[VoiceMessages] Chat %d: начинаю генерацию голосового сообщения с заданным текстом", chatID)

	if vms.bot.config.Debug {
		log.Printf("[DEBUG][VoiceMessages] Chat %d: текст (%d символов): %s",
			chatID, len(text), text)
	}

	// Оптимизируем текст для голоса
	voiceOptimizedText := vms.optimizeTextForVoice(text)

	// Генерируем аудио через ElevenLabs
	audioData, err := vms.elevenLabs.GenerateAudio(voiceOptimizedText)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка генерации аудио: %v", chatID, err)
		return
	}

	// Конвертируем MP3 в OGG/Opus для Telegram
	voiceData, err := vms.convertToVoiceFormat(audioData)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка конвертации аудио: %v", chatID, err)
		return
	}

	// Отправляем голосовое сообщение
	if err := vms.sendVoiceMessage(chatID, voiceData); err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка отправки голосового сообщения: %v", chatID, err)
		return
	}

	duration := time.Since(startTime)
	usage, limit, plan := vms.elevenLabs.GetUsageInfo()
	log.Printf("[VoiceMessages] Chat %d: голосовое сообщение с заданным текстом отправлено за %s. Использовано: %d/%d (%s)",
		chatID, duration.Round(time.Millisecond), usage, limit, plan)
}

// generateAndSendVoiceMessageReply с заданным текстом и reply для Free Will
func (vms *VoiceMessageService) generateAndSendVoiceMessageReply(chatID int64, text string, replyToMessageID int) {
	if !vms.enabled {
		log.Printf("[VoiceMessages] Chat %d: сервис отключен", chatID)
		return
	}

	startTime := time.Now()
	log.Printf("[VoiceMessages] Chat %d: начинаю генерацию голосового ответа с заданным текстом (reply to %d)", chatID, replyToMessageID)

	if vms.bot.config.Debug {
		log.Printf("[DEBUG][VoiceMessages] Chat %d: текст ответа (%d символов): %s",
			chatID, len(text), text)
	}

	// Оптимизируем текст для голоса
	voiceOptimizedText := vms.optimizeTextForVoice(text)

	// Генерируем аудио через ElevenLabs
	audioData, err := vms.elevenLabs.GenerateAudio(voiceOptimizedText)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка генерации аудио: %v", chatID, err)
		return
	}

	// Конвертируем MP3 в OGG/Opus для Telegram
	voiceData, err := vms.convertToVoiceFormat(audioData)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка конвертации аудио: %v", chatID, err)
		return
	}

	// Отправляем голосовое сообщение как reply
	if err := vms.sendVoiceMessageReply(chatID, voiceData, replyToMessageID); err != nil {
		log.Printf("[ERROR][VoiceMessages] Chat %d: ошибка отправки голосового ответа: %v", chatID, err)
		return
	}

	duration := time.Since(startTime)
	usage, limit, plan := vms.elevenLabs.GetUsageInfo()
	log.Printf("[VoiceMessages] Chat %d: голосовой ответ с заданным текстом отправлен за %s. Использовано: %d/%d (%s)",
		chatID, duration.Round(time.Millisecond), usage, limit, plan)
}

// sendVoiceMessageReply отправляет голосовое сообщение как ответ в Telegram
func (vms *VoiceMessageService) sendVoiceMessageReply(chatID int64, voiceData []byte, replyToMessageID int) error {
	// Создаем FileBytes для отправки
	voiceMsg := tgbotapi.NewVoice(chatID, tgbotapi.FileBytes{
		Name:  "voice_message.ogg",
		Bytes: voiceData,
	})

	// Устанавливаем reply
	voiceMsg.ReplyToMessageID = replyToMessageID

	// Отправляем через Telegram API
	sentMsg, err := vms.bot.api.Send(voiceMsg)
	if err != nil {
		return fmt.Errorf("ошибка отправки в Telegram: %w", err)
	}

	// Сохраняем сообщение в истории
	if vms.bot.storage != nil {
		vms.bot.storage.AddMessage(chatID, &sentMsg)
		if vms.bot.config.Debug {
			log.Printf("[DEBUG][VoiceMessages] Голосовое сообщение-ответ сохранено в историю")
		}
	}

	// Запускаем транскрибацию голосового сообщения бота в отдельной горутине
	go func() {
		if vms.bot.config.Debug {
			log.Printf("[DEBUG][VoiceMessages] Chat %d: Запуск транскрибации голосового ответа бота ID %d", chatID, sentMsg.MessageID)
		}

		// Вызываем handleVoiceMessage для транскрибации собственного голосового сообщения
		vms.bot.handleVoiceMessage(&sentMsg)
	}()

	return nil
}

// generateAndSendVoiceMessageForFreeWill специальный метод для Free Will, игнорирующий VOICE_MESSAGES_ENABLED
func (vms *VoiceMessageService) generateAndSendVoiceMessageForFreeWill(chatID int64, text string) {
	// Проверяем только доступность ElevenLabs клиента, а не глобальный enabled флаг
	if vms.elevenLabs == nil {
		log.Printf("[VoiceMessages][FreeWill] Chat %d: ElevenLabs клиент недоступен", chatID)
		return
	}

	startTime := time.Now()
	log.Printf("[VoiceMessages][FreeWill] Chat %d: начинаю генерацию голосового сообщения Free Will с заданным текстом", chatID)

	if vms.bot.config.Debug {
		log.Printf("[DEBUG][VoiceMessages][FreeWill] Chat %d: текст (%d символов): %s",
			chatID, len(text), text)
	}

	// Оптимизируем текст для голоса
	voiceOptimizedText := vms.optimizeTextForVoice(text)

	// Генерируем аудио через ElevenLabs
	audioData, err := vms.elevenLabs.GenerateAudio(voiceOptimizedText)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages][FreeWill] Chat %d: ошибка генерации аудио: %v", chatID, err)
		return
	}

	// Конвертируем MP3 в OGG/Opus для Telegram
	voiceData, err := vms.convertToVoiceFormat(audioData)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages][FreeWill] Chat %d: ошибка конвертации аудио: %v", chatID, err)
		return
	}

	// Отправляем голосовое сообщение
	if err := vms.sendVoiceMessage(chatID, voiceData); err != nil {
		log.Printf("[ERROR][VoiceMessages][FreeWill] Chat %d: ошибка отправки голосового сообщения: %v", chatID, err)
		return
	}

	duration := time.Since(startTime)
	usage, limit, plan := vms.elevenLabs.GetUsageInfo()
	log.Printf("[VoiceMessages][FreeWill] Chat %d: голосовое сообщение Free Will отправлено за %s. Использовано: %d/%d (%s)",
		chatID, duration.Round(time.Millisecond), usage, limit, plan)
}

// generateAndSendVoiceMessageReplyForFreeWill специальный метод для Free Will с reply, игнорирующий VOICE_MESSAGES_ENABLED
func (vms *VoiceMessageService) generateAndSendVoiceMessageReplyForFreeWill(chatID int64, text string, replyToMessageID int) {
	// Проверяем только доступность ElevenLabs клиента, а не глобальный enabled флаг
	if vms.elevenLabs == nil {
		log.Printf("[VoiceMessages][FreeWill] Chat %d: ElevenLabs клиент недоступен", chatID)
		return
	}

	startTime := time.Now()
	log.Printf("[VoiceMessages][FreeWill] Chat %d: начинаю генерацию голосового ответа Free Will с заданным текстом (reply to %d)", chatID, replyToMessageID)

	if vms.bot.config.Debug {
		log.Printf("[DEBUG][VoiceMessages][FreeWill] Chat %d: текст ответа (%d символов): %s",
			chatID, len(text), text)
	}

	// Оптимизируем текст для голоса
	voiceOptimizedText := vms.optimizeTextForVoice(text)

	// Генерируем аудио через ElevenLabs
	audioData, err := vms.elevenLabs.GenerateAudio(voiceOptimizedText)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages][FreeWill] Chat %d: ошибка генерации аудио: %v", chatID, err)
		return
	}

	// Конвертируем MP3 в OGG/Opus для Telegram
	voiceData, err := vms.convertToVoiceFormat(audioData)
	if err != nil {
		log.Printf("[ERROR][VoiceMessages][FreeWill] Chat %d: ошибка конвертации аудио: %v", chatID, err)
		return
	}

	// Отправляем голосовое сообщение как reply
	if err := vms.sendVoiceMessageReply(chatID, voiceData, replyToMessageID); err != nil {
		log.Printf("[ERROR][VoiceMessages][FreeWill] Chat %d: ошибка отправки голосового ответа: %v", chatID, err)
		return
	}

	duration := time.Since(startTime)
	usage, limit, plan := vms.elevenLabs.GetUsageInfo()
	log.Printf("[VoiceMessages][FreeWill] Chat %d: голосовой ответ Free Will отправлен за %s. Использовано: %d/%d (%s)",
		chatID, duration.Round(time.Millisecond), usage, limit, plan)
}
