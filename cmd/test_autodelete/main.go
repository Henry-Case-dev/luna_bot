package main

import (
	"context"
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	log.Println("🧪 Тестирование автоудаления технических сообщений")

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

	log.Printf("📊 Настройки автоудаления:")
	log.Printf("   ERROR_MESSAGE_AUTO_DELETE_SECONDS: %d", cfg.ErrorMessageAutoDeleteSeconds)
	log.Printf("   DEBUG: %v", cfg.Debug)

	if cfg.ErrorMessageAutoDeleteSeconds <= 0 {
		log.Println("⚠️  ПРЕДУПРЕЖДЕНИЕ: Автоудаление сообщений об ошибках отключено (значение <= 0)")
	} else {
		log.Printf("✅ Автоудаление сообщений об ошибках включено (%d секунд)", cfg.ErrorMessageAutoDeleteSeconds)
	}

	log.Println("✅ Проверка конфигурации завершена")
}
