package main

import (
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	log.Println("🎤 Тестирование настроек транскрибации голосовых сообщений")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	log.Printf("📊 Настройки для голосовых сообщений:")
	log.Printf("   VoiceTranscriptionEnabledDefault: %v", cfg.VoiceTranscriptionEnabledDefault)
	log.Printf("   StorageType: %s", cfg.StorageType)
	log.Printf("   MongoDB URI: %s", maskString(cfg.MongoDbURI))
	log.Printf("   MongoDB Database: %s", cfg.MongoDbName)

	log.Printf("📊 Настройки ElevenLabs:")
	log.Printf("   API Key настроен: %v", cfg.ElevenLabsAPIKey != "")
	log.Printf("   Voice ID: %s", cfg.ElevenLabsVoiceID)
	log.Printf("   Model: %s", cfg.ElevenLabsModel)
	log.Printf("   Plan: %s", cfg.ElevenLabsPlan)
	log.Printf("   MinVoiceMessages: %d", cfg.MinVoiceMessages)
	log.Printf("   MaxVoiceMessages: %d", cfg.MaxVoiceMessages)

	log.Printf("📊 Настройки транскрибации:")
	if len(cfg.VoiceFormatPrompt) > 0 {
		log.Printf("   VOICE_FORMAT_PROMPT: %s...", cfg.VoiceFormatPrompt[:min(100, len(cfg.VoiceFormatPrompt))])
	} else {
		log.Printf("   VOICE_FORMAT_PROMPT: не настроен")
	}

	log.Printf("📊 Настройки LLM:")
	log.Printf("   Provider: %s", cfg.LLMProvider)
	log.Printf("   Gemini API Key настроен: %v", cfg.GeminiAPIKey != "")
	log.Printf("   Model: %s", cfg.GeminiModelName)

	log.Println("✅ Проверка настроек завершена. Проверьте логи бота для подтверждения работы транскрибации.")
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "***masked***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
