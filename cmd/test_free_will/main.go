package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Henry-Case-dev/luna_bot/internal/bot"
	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	source := config.NewYAMLConfigSource("configs/luna_bot.yaml")
	source.SetStrictMode(false)
	cfgV2, yamlErr := source.Load(context.Background())

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	if yamlErr != nil {
		log.Printf("[WARN] YAML не загружен (%v), используется .env", yamlErr)
	} else {
		log.Printf("[INFO] YAML загружен успешно")
		_ = cfgV2
	}

	// Принудительно включаем Free Will для тестирования
	cfg.FreeWillEnabled = true
	cfg.Debug = true

	log.Println("=== ТЕСТИРОВАНИЕ FREE WILL ===")
	log.Printf("Free Will включен: %t", cfg.FreeWillEnabled)
	log.Printf("Минимальный интервал: %.2f минут", cfg.FreeWillMinIntervalMinutes)
	log.Printf("Максимальный интервал: %.2f минут", cfg.FreeWillMaxIntervalMinutes)
	log.Printf("Контекстное окно: %d сообщений", cfg.FreeWillContextWindow)
	log.Printf("Максимум решений в час: %d", cfg.FreeWillMaxDecisionsPerHour)

	botInstance, err := bot.New(cfg, nil)
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	log.Println("Запуск тестирования Free Will...")

	if err := botInstance.Start(); err != nil {
		log.Fatalf("Ошибка запуска бота: %v", err)
	}

	log.Println("Бот запущен. Для тестирования используйте:")
	log.Println("  /freewill_status - статус модуля")
	log.Println("  /freewill_toggle - переключить включение/выключение")
	log.Println("  /freewill_force - принудительный анализ")
	log.Println("  /freewill_mood - текущее настроение")
	log.Println("  /freewill_mood update - обновить настроение")

	// Ожидание сигнала остановки
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Остановка бота...")
	botInstance.Stop()
}
