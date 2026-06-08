package main

import (
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	log.Println("🧪 Тестирование автоудаления технических сообщений")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
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
