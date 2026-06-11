package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	fmt.Println("🧪 Тестирование системы модерации")

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

	fmt.Printf("📊 Настройки модерации:\n")
	fmt.Printf("   MOD_INTERVAL: %d сообщений\n", cfg.ModInterval)
	fmt.Printf("   MOD_CHECK_ADMIN_RIGHTS: %v\n", cfg.ModCheckAdminRights)
	fmt.Printf("   MOD_PURGE_DELAY_DURATION: %v\n", cfg.ModPurgeDelay)
	fmt.Printf("   MOD_PURGE_WINDOW_DURATION: %v\n", cfg.ModPurgeDuration)
	fmt.Printf("   MOD_MUTE_TIME_MIN: %d минут\n", cfg.ModMuteTimeMin)
	fmt.Printf("   MOD_BAN_TIME_MIN: %d минут\n", cfg.ModBanTimeMin)
	fmt.Printf("   Правил модерации: %d\n", len(cfg.ModRules))

	if cfg.ModInterval <= 0 {
		fmt.Println("⚠️  ПРЕДУПРЕЖДЕНИЕ: Неверный интервал модерации (MOD_INTERVAL <= 0)")
	}

	// Показываем настройки оптимизации
	fmt.Printf("\n🔧 Оптимизация модерации:\n")
	fmt.Printf("   Максимальный возраст сообщений для удаления: 48 часов (Telegram лимит)\n")
	fmt.Printf("   Время жизни кэша удаленных сообщений: 1 час\n")
	fmt.Printf("   Интервал очистки кэша: 15 минут\n")

	// Анализируем правила модерации
	if len(cfg.ModRules) > 0 {
		fmt.Printf("\n📋 Правила модерации:\n")
		for i, rule := range cfg.ModRules {
			fmt.Printf("   %d. %s:\n", i+1, rule.RuleName)
			fmt.Printf("      ChatID: %s\n", rule.ChatID)
			fmt.Printf("      UserID: %s\n", rule.UserID)
			fmt.Printf("      Наказание: %s\n", rule.Punishment)
			if len(rule.Keywords) > 0 {
				fmt.Printf("      Ключевые слова: %v\n", rule.Keywords)
			}
			if rule.LLMInstruction != "" && rule.LLMInstruction != "none" {
				fmt.Printf("      LLM инструкция: %s\n", rule.LLMInstruction)
			}
			if rule.PunishmentNote != "" {
				fmt.Printf("      Примечание: %s\n", rule.PunishmentNote)
			}
			if rule.ReplacementText != "" {
				fmt.Printf("      Текст замены: %s\n", rule.ReplacementText)
			}
			fmt.Printf("      Уведомление в чат: %v\n", rule.NotifyChat)
			fmt.Printf("      Уведомление пользователя: %v\n", rule.NotifyUser)
		}
	} else {
		fmt.Println("⚠️  ПРЕДУПРЕЖДЕНИЕ: Правила модерации не настроены")
	}

	// Проверяем настройки задержки очистки
	if cfg.ModPurgeDelay > 0 {
		fmt.Printf("\n⏰ Задержка перед очисткой сообщений: %v\n", cfg.ModPurgeDelay)
		fmt.Println("   ✅ Задержка настроена - пользователи смогут увидеть предупреждение")
	} else {
		fmt.Println("\n⚠️  Задержка перед очисткой не настроена (мгновенное удаление)")
	}

	// Проверяем права администратора
	if cfg.ModCheckAdminRights {
		fmt.Println("\n🔒 Проверка прав администратора включена")
		fmt.Println("   ✅ Модерация будет работать только если бот имеет права администратора")
	} else {
		fmt.Println("\n⚠️  Проверка прав администратора отключена")
		fmt.Println("   ⚠️  Модерация может не работать правильно без прав администратора")
	}

	fmt.Println("\n🎯 Рекомендации по оптимизации:")
	fmt.Println("   • Система кэширования должна значительно уменьшить количество неудачных API запросов")
	fmt.Println("   • Старые сообщения (>48 часов) автоматически пропускаются")
	fmt.Println("   • Логирование оптимизировано - меньше спама в логах")
	fmt.Println("   • Задержка между удалениями сообщений: 200ms (для предотвращения rate limiting)")

	fmt.Println("\n✅ Проверка конфигурации модерации завершена")
}
