package elevenlabs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ElevenLabsPlan определяет тарифный план
type ElevenLabsPlan string

const (
	PlanFree    ElevenLabsPlan = "free"
	PlanStarter ElevenLabsPlan = "starter"
	PlanCreator ElevenLabsPlan = "creator"
	PlanPro     ElevenLabsPlan = "pro"
)

// PlanLimits содержит лимиты кредитов для каждого плана
var PlanLimits = map[ElevenLabsPlan]int{
	PlanFree:    10000,
	PlanStarter: 30000,
	PlanCreator: 100000,
	PlanPro:     500000,
}

// DailyLimitTracker отслеживает дневные лимиты использования
type DailyLimitTracker struct {
	Plan             ElevenLabsPlan
	MonthlyCredits   int
	DailyLimit       int
	CurrentDayUsage  int
	LastResetDate    time.Time
	EstimatedCredits int // Примерная стоимость одного запроса
}

// TTSRequest представляет запрос на генерацию речи
type TTSRequest struct {
	Text          string        `json:"text"`
	ModelID       string        `json:"model_id"`
	VoiceSettings VoiceSettings `json:"voice_settings"`
}

// VoiceSettings настройки голоса
type VoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style,omitempty"`
	UseSpeakerBoost bool    `json:"use_speaker_boost,omitempty"`
	Speed           float64 `json:"speed,omitempty"` // Скорость речи: 0.7-1.2, по умолчанию 1.0
}

// Client клиент ElevenLabs API
type Client struct {
	APIKey       string
	BaseURL      string
	VoiceID      string
	Model        string
	HTTPClient   *http.Client
	LimitTracker *DailyLimitTracker
	Debug        bool
	// Настройки голоса
	VoiceSettings VoiceSettings
	// Промпт-настройки
	StylePrompt   string
	EmotionPrompt string
	PacePrompt    string
	RandomVoice   bool
}

// VoiceConfig содержит настройки голоса для инициализации клиента
type VoiceConfig struct {
	Stability       float64
	SimilarityBoost float64
	Style           float64
	UseSpeakerBoost bool
	Speed           float64 // Скорость речи: 0.7-1.2, по умолчанию 1.0
	StylePrompt     string
	EmotionPrompt   string
	PacePrompt      string
	RandomVoice     bool
}

// NewClient создает новый клиент ElevenLabs
func NewClient(apiKey, voiceID, model string, plan ElevenLabsPlan, debug bool) *Client {
	return NewClientWithVoiceConfig(apiKey, voiceID, model, plan, debug, VoiceConfig{
		Stability:       0.5,
		SimilarityBoost: 0.8,
		Style:           0.0,
		UseSpeakerBoost: true,
		Speed:           1.0, // Нормальная скорость по умолчанию
	})
}

// NewClientWithVoiceConfig создает новый клиент ElevenLabs с настройками голоса
func NewClientWithVoiceConfig(apiKey, voiceID, model string, plan ElevenLabsPlan, debug bool, voiceConfig VoiceConfig) *Client {
	monthlyCredits, exists := PlanLimits[plan]
	if !exists {
		monthlyCredits = PlanLimits[PlanFree] // По умолчанию Free план
	}

	// Рассчитываем дневной лимит (месячный лимит / 30 дней)
	dailyLimit := monthlyCredits / 30
	estimatedCredits := 100 // Примерная стоимость одного голосового сообщения

	tracker := &DailyLimitTracker{
		Plan:             plan,
		MonthlyCredits:   monthlyCredits,
		DailyLimit:       dailyLimit,
		CurrentDayUsage:  0,
		LastResetDate:    time.Now().UTC().Truncate(24 * time.Hour),
		EstimatedCredits: estimatedCredits,
	}

	// Валидация скорости согласно документации ElevenLabs
	speed := voiceConfig.Speed
	if speed < 0.7 {
		log.Printf("[WARN][ElevenLabs] Speed %.2f ниже минимума 0.7, устанавливаю 0.7", speed)
		speed = 0.7
	} else if speed > 1.2 {
		log.Printf("[WARN][ElevenLabs] Speed %.2f выше максимума 1.2, устанавливаю 1.2", speed)
		speed = 1.2
	} else if speed == 0.0 {
		speed = 1.0 // По умолчанию если не задано
	}

	voiceSettings := VoiceSettings{
		Stability:       voiceConfig.Stability,
		SimilarityBoost: voiceConfig.SimilarityBoost,
		Style:           voiceConfig.Style,
		UseSpeakerBoost: voiceConfig.UseSpeakerBoost,
		Speed:           speed,
	}

	return &Client{
		APIKey:  apiKey,
		BaseURL: "https://api.elevenlabs.io/v1",
		VoiceID: voiceID,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		LimitTracker:  tracker,
		Debug:         debug,
		VoiceSettings: voiceSettings,
		StylePrompt:   voiceConfig.StylePrompt,
		EmotionPrompt: voiceConfig.EmotionPrompt,
		PacePrompt:    voiceConfig.PacePrompt,
		RandomVoice:   voiceConfig.RandomVoice,
	}
}

// CanSendVoiceMessage проверяет, можем ли мы отправить голосовое сообщение сегодня
func (c *Client) CanSendVoiceMessage() bool {
	c.resetDailyUsageIfNeeded()

	if c.Debug {
		log.Printf("[DEBUG][ElevenLabs] Plan: %s, Daily usage: %d/%d",
			c.LimitTracker.Plan, c.LimitTracker.CurrentDayUsage, c.LimitTracker.DailyLimit)
	}

	return c.LimitTracker.CurrentDayUsage+c.LimitTracker.EstimatedCredits <= c.LimitTracker.DailyLimit
}

// resetDailyUsageIfNeeded сбрасывает дневное использование если прошел день
func (c *Client) resetDailyUsageIfNeeded() {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	if now.After(c.LimitTracker.LastResetDate) {
		if c.Debug {
			log.Printf("[DEBUG][ElevenLabs] Сброс дневного лимита. Было: %d, стало: 0",
				c.LimitTracker.CurrentDayUsage)
		}
		c.LimitTracker.CurrentDayUsage = 0
		c.LimitTracker.LastResetDate = now
	}
}

// GenerateAudio генерирует аудио из текста
func (c *Client) GenerateAudio(text string) ([]byte, error) {
	if !c.CanSendVoiceMessage() {
		return nil, fmt.Errorf("превышен дневной лимит: %d/%d кредитов",
			c.LimitTracker.CurrentDayUsage, c.LimitTracker.DailyLimit)
	}

	// Применяем промпт-настройки к тексту
	processedText := c.applyVoicePrompts(text)

	if c.Debug {
		log.Printf("[DEBUG][ElevenLabs] Генерация аудио для текста: %s", processedText)
	}

	// Подготавливаем запрос с настройками голоса из клиента
	ttsRequest := TTSRequest{
		Text:          processedText,
		ModelID:       c.Model,
		VoiceSettings: c.VoiceSettings,
	}

	jsonData, err := json.Marshal(ttsRequest)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	// Создаем HTTP запрос
	url := fmt.Sprintf("%s/text-to-speech/%s", c.BaseURL, c.VoiceID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.APIKey)

	if c.Debug {
		log.Printf("[DEBUG][ElevenLabs] Отправка запроса на %s", url)
		log.Printf("[DEBUG][ElevenLabs] Voice settings: stability=%.2f, similarity=%.2f, style=%.2f, boost=%t, speed=%.2f",
			c.VoiceSettings.Stability, c.VoiceSettings.SimilarityBoost,
			c.VoiceSettings.Style, c.VoiceSettings.UseSpeakerBoost, c.VoiceSettings.Speed)
	}

	// Выполняем запрос
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	// Читаем аудио данные
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения аудио данных: %w", err)
	}

	// Обновляем счетчик использования
	c.LimitTracker.CurrentDayUsage += c.LimitTracker.EstimatedCredits

	if c.Debug {
		log.Printf("[DEBUG][ElevenLabs] Получено аудио: %d байт. Использовано кредитов: %d/%d",
			len(audioData), c.LimitTracker.CurrentDayUsage, c.LimitTracker.DailyLimit)
	}

	return audioData, nil
}

// applyVoicePrompts применяет промпт-настройки к тексту
func (c *Client) applyVoicePrompts(text string) string {
	var prompts []string

	// Добавляем промпты в порядке приоритета: стиль → эмоции → темп
	if c.StylePrompt != "" {
		prompts = append(prompts, c.StylePrompt)
	}
	if c.EmotionPrompt != "" {
		prompts = append(prompts, c.EmotionPrompt)
	}
	if c.PacePrompt != "" {
		prompts = append(prompts, c.PacePrompt)
	}

	if len(prompts) == 0 {
		return text
	}

	// Формируем инструкцию для ElevenLabs
	var instruction strings.Builder
	instruction.WriteString("(")
	for i, prompt := range prompts {
		if i > 0 {
			instruction.WriteString(", ")
		}
		instruction.WriteString(prompt)
	}
	instruction.WriteString(") ")
	instruction.WriteString(text)

	return instruction.String()
}

// GetUsageInfo возвращает информацию об использовании
func (c *Client) GetUsageInfo() (int, int, string) {
	c.resetDailyUsageIfNeeded()
	return c.LimitTracker.CurrentDayUsage, c.LimitTracker.DailyLimit, string(c.LimitTracker.Plan)
}

// GetRemainingCredits возвращает оставшиеся кредиты на сегодня
func (c *Client) GetRemainingCredits() int {
	c.resetDailyUsageIfNeeded()
	remaining := c.LimitTracker.DailyLimit - c.LimitTracker.CurrentDayUsage
	if remaining < 0 {
		return 0
	}
	return remaining
}
