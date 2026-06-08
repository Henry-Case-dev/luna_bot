package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("=== ТЕСТИРОВАНИЕ ЗАГРУЗКИ ПЕРЕМЕННЫХ ОКРУЖЕНИЯ ===")

	// Загружаем .env файлы как в main.go
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("[WARN] Ошибка загрузки .env: %v", err)
	} else {
		fmt.Println("✅ .env файл загружен")
	}

	if err := godotenv.Load(".env.secrets"); err != nil {
		log.Printf("[WARN] Ошибка загрузки .env.secrets: %v", err)
	} else {
		fmt.Println("✅ .env.secrets файл загружен")
	}

	// Проверяем некоторые переменные напрямую
	fmt.Printf("\n=== ПРОВЕРКА ПЕРЕМЕННЫХ ОКРУЖЕНИЯ ===\n")
	fmt.Printf("TELEGRAM_TOKEN: %s\n", maskToken(os.Getenv("TELEGRAM_TOKEN")))
	fmt.Printf("GEMINI_API_KEY: %s\n", maskToken(os.Getenv("GEMINI_API_KEY")))
	fmt.Printf("ELEVENLABS_API_KEY: %s\n", maskToken(os.Getenv("ELEVENLABS_API_KEY")))
	fmt.Printf("LLM_PROVIDER: %s\n", os.Getenv("LLM_PROVIDER"))
	fmt.Printf("FREE_WILL_ENABLED: %s\n", os.Getenv("FREE_WILL_ENABLED"))
	fmt.Printf("VOICE_MESSAGES_ENABLED: %s\n", os.Getenv("VOICE_MESSAGES_ENABLED"))

	// Тестируем загрузку конфигурации
	fmt.Printf("\n=== ТЕСТИРОВАНИЕ CONFIG.LOAD ===\n")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	fmt.Printf("✅ Конфигурация загружена успешно\n")
	fmt.Printf("Telegram Token установлен: %t\n", cfg.TelegramToken != "")
	fmt.Printf("Gemini API Key установлен: %t\n", cfg.GeminiAPIKey != "")
	fmt.Printf("ElevenLabs API Key установлен: %t\n", cfg.ElevenLabsAPIKey != "")
	fmt.Printf("LLM Provider: %s\n", cfg.LLMProvider)
	fmt.Printf("Free Will включен: %t\n", cfg.FreeWillEnabled)
	fmt.Printf("Voice Messages включен: %t\n", cfg.VoiceMessagesEnabled)
	fmt.Printf("Storage Type: %s\n", cfg.StorageType)

	fmt.Printf("\n=== ✅ ВСЕ ТЕСТЫ ПРОЙДЕНЫ ===\n")
}

func maskToken(token string) string {
	if token == "" {
		return "(не установлен)"
	}
	if len(token) < 10 {
		return "*****"
	}
	return token[:5] + "..." + token[len(token)-5:]
}
