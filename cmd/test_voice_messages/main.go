package main

import (
	"context"
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/elevenlabs"
)

func main() {
	log.Println("🧪 Тестирование голосовых сообщений ElevenLabs")

	source := config.NewYAMLConfigSource("configs/luna_bot.yaml")
	source.SetStrictMode(false)
	cfgV2, yamlErr := source.Load(context.Background())

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	if yamlErr != nil {
		log.Printf("[WARN] YAML не загружен (%v), используется .env", yamlErr)
	} else {
		log.Printf("[INFO] YAML загружен успешно")
		_ = cfgV2
	}

	log.Printf("📊 Настройки ElevenLabs:")
	log.Printf("   API Key: %s", maskAPIKey(cfg.ElevenLabsAPIKey))
	log.Printf("   Voice ID: %s", cfg.ElevenLabsVoiceID)
	log.Printf("   Model: %s", cfg.ElevenLabsModel)
	log.Printf("   Plan: %s", cfg.ElevenLabsPlan)
	log.Printf("   Voice Message Prompt: %s", truncateString(cfg.VoiceMessagesPrompt, 100))
	log.Printf("   Min Messages: %d", cfg.MinVoiceMessages)
	log.Printf("   Max Messages: %d", cfg.MaxVoiceMessages)
	log.Printf("   Temp Directory: %s", cfg.VoiceMessageTempDir)

	log.Printf("📊 Настройки качества голоса:")
	log.Printf("   Stability: %.2f", cfg.ElevenLabsStability)
	log.Printf("   Similarity Boost: %.2f", cfg.ElevenLabsSimilarityBoost)
	log.Printf("   Style: %.2f", cfg.ElevenLabsStyle)
	log.Printf("   Use Speaker Boost: %t", cfg.ElevenLabsUseSpeakerBoost)

	log.Printf("📊 Промпт-настройки:")
	log.Printf("   Style Prompt: %s", truncateString(cfg.ElevenLabsStylePrompt, 50))
	log.Printf("   Emotion Prompt: %s", truncateString(cfg.ElevenLabsEmotionPrompt, 50))
	log.Printf("   Pace Prompt: %s", truncateString(cfg.ElevenLabsPacePrompt, 50))
	log.Printf("   Random Voice: %t", cfg.ElevenLabsRandomVoice)

	// Проверяем настройки ElevenLabs
	if cfg.ElevenLabsAPIKey == "" {
		log.Println("⚠️  ПРЕДУПРЕЖДЕНИЕ: ElevenLabs API ключ не установлен")
	} else {
		log.Println("✅ ElevenLabs API ключ установлен")
	}

	// Проверяем тарифный план
	plan := elevenlabs.ElevenLabsPlan(cfg.ElevenLabsPlan)
	if limit, exists := elevenlabs.PlanLimits[plan]; exists {
		dailyLimit := limit / 30
		log.Printf("✅ План %s: %d кредитов/месяц, ~%d кредитов/день", cfg.ElevenLabsPlan, limit, dailyLimit)
	} else {
		log.Printf("⚠️  Неизвестный план: %s", cfg.ElevenLabsPlan)
	}

	// Тестируем создание клиента ElevenLabs с новыми настройками
	if cfg.ElevenLabsAPIKey != "" {
		voiceConfig := elevenlabs.VoiceConfig{
			Stability:       cfg.ElevenLabsStability,
			SimilarityBoost: cfg.ElevenLabsSimilarityBoost,
			Style:           cfg.ElevenLabsStyle,
			UseSpeakerBoost: cfg.ElevenLabsUseSpeakerBoost,
			StylePrompt:     cfg.ElevenLabsStylePrompt,
			EmotionPrompt:   cfg.ElevenLabsEmotionPrompt,
			PacePrompt:      cfg.ElevenLabsPacePrompt,
			RandomVoice:     cfg.ElevenLabsRandomVoice,
		}

		client := elevenlabs.NewClientWithVoiceConfig(
			cfg.ElevenLabsAPIKey,
			cfg.ElevenLabsVoiceID,
			cfg.ElevenLabsModel,
			plan,
			cfg.Debug,
			voiceConfig,
		)

		log.Printf("✅ ElevenLabs клиент создан успешно")

		// Проверяем лимиты
		usage, limit, planName := client.GetUsageInfo()
		remaining := client.GetRemainingCredits()

		log.Printf("📊 Статистика использования:")
		log.Printf("   Использовано сегодня: %d/%d кредитов", usage, limit)
		log.Printf("   Осталось сегодня: %d кредитов", remaining)
		log.Printf("   Текущий план: %s", planName)

		if client.CanSendVoiceMessage() {
			log.Printf("✅ Можно отправлять голосовые сообщения")

			// Тест применения промптов
			testText := "Привет, это тест голосового сообщения!"
			log.Printf("🧪 Тестируем применение промптов к тексту: '%s'", testText)

			// Генерируем только обработанный текст без реального API вызова
			log.Printf("📝 Настройки голоса будут применены автоматически:")
			log.Printf("   Stability: %.2f", client.VoiceSettings.Stability)
			log.Printf("   Similarity Boost: %.2f", client.VoiceSettings.SimilarityBoost)
			log.Printf("   Style: %.2f", client.VoiceSettings.Style)
			log.Printf("   Speaker Boost: %t", client.VoiceSettings.UseSpeakerBoost)

			if client.StylePrompt != "" || client.EmotionPrompt != "" || client.PacePrompt != "" {
				log.Printf("📝 Промпты будут добавлены к тексту:")
				if client.StylePrompt != "" {
					log.Printf("   Style: %s", client.StylePrompt)
				}
				if client.EmotionPrompt != "" {
					log.Printf("   Emotion: %s", client.EmotionPrompt)
				}
				if client.PacePrompt != "" {
					log.Printf("   Pace: %s", client.PacePrompt)
				}
			} else {
				log.Printf("📝 Промпты не установлены, будет использован исходный текст")
			}
		} else {
			log.Printf("❌ Нельзя отправлять голосовые сообщения (превышен лимит)")
		}
	}

	log.Println("✅ Проверка голосовых сообщений завершена")
}

func maskAPIKey(key string) string {
	if len(key) < 8 {
		return "****"
	}
	return key[:4] + repeat("*", len(key)-8) + key[len(key)-4:]
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Простая реализация repeat для строк
func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
