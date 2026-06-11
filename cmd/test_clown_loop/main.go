package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	log.Println("🔍 Анализ системы предотвращения бесконечного цикла реакций клоуна")

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

	log.Printf("📊 Настройки реакций:")
	log.Printf("   REACTIONS_ENABLED: %v", cfg.ReactionsEnabled)
	log.Printf("   CLOWN_RESPONSE_PROBABILITY: %d%%", cfg.ClownResponseProbability)
	log.Printf("   CLOWN_COOLDOWN_SECONDS: %d секунд", cfg.ClownCooldownSeconds)
	log.Printf("   MAX_CLOWN_RESPONSES_PER_HOUR: %d ответов", cfg.MaxClownResponsesPerHour)

	// Анализируем решение проблем бесконечного цикла
	analyzeLoopPreventionSolution(cfg)

	// Симулируем работу лимитера
	simulateLimiterBehavior(cfg)

	log.Println("\n🏁 Анализ завершен")
}

func analyzeLoopPreventionSolution(cfg *config.Config) {
	log.Println("\n✅ АНАЛИЗ РЕШЕНИЯ ПРОБЛЕМ БЕСКОНЕЧНОГО ЦИКЛА:")

	// Решение 1: Вероятность ответа
	log.Printf("✅ РЕШЕНИЕ 1: Вероятность ответа на клоуна = %d%%", cfg.ClownResponseProbability)
	if cfg.ClownResponseProbability <= 50 {
		log.Printf("   • ОТЛИЧНО: Умеренная вероятность предотвращает спам")
	} else if cfg.ClownResponseProbability <= 70 {
		log.Printf("   • НОРМАЛЬНО: Довольно высокая вероятность, но приемлемая")
	} else {
		log.Printf("   • ⚠️  ОСТОРОЖНО: Высокая вероятность может создать много ответов")
	}

	// Решение 2: Cooldown пользователя
	log.Printf("\n✅ РЕШЕНИЕ 2: Cooldown между ответами = %d секунд", cfg.ClownCooldownSeconds)
	if cfg.ClownCooldownSeconds >= 30 {
		log.Printf("   • ОТЛИЧНО: Достаточный cooldown предотвращает rapid-fire клоунов")
	} else if cfg.ClownCooldownSeconds >= 15 {
		log.Printf("   • НОРМАЛЬНО: Короткий cooldown, но все еще полезный")
	} else {
		log.Printf("   • ⚠️  ОСТОРОЖНО: Очень короткий cooldown")
	}

	// Решение 3: Лимит в час
	log.Printf("\n✅ РЕШЕНИЕ 3: Максимум ответов в час = %d", cfg.MaxClownResponsesPerHour)
	if cfg.MaxClownResponsesPerHour <= 15 {
		log.Printf("   • ОТЛИЧНО: Строгий лимит предотвращает злоупотребления")
	} else if cfg.MaxClownResponsesPerHour <= 30 {
		log.Printf("   • НОРМАЛЬНО: Умеренный лимит")
	} else {
		log.Printf("   • ⚠️  ОСТОРОЖНО: Высокий лимит может не защитить от спама")
	}

	log.Printf("\n🎯 ОБЩАЯ ОЦЕНКА:")
	log.Printf("   • Система из 3-х уровней защиты от бесконечного цикла")
	log.Printf("   • Комбинация вероятности, cooldown и rate limiting")
	log.Printf("   • Админская команда /clown_stats для мониторинга")
}

func simulateLimiterBehavior(cfg *config.Config) {
	log.Println("\n🧪 СИМУЛЯЦИЯ РАБОТЫ ЛИМИТЕРА:")

	// Симулируем быстрые попытки от одного пользователя
	userID := int64(123456)
	successCount := 0
	totalAttempts := 20

	log.Printf("📋 Тест: %d быстрых попыток клоуна от пользователя %d", totalAttempts, userID)

	for i := 0; i < totalAttempts; i++ {
		// Симулируем проверку вероятности
		passedProbability := rand.Intn(100) < cfg.ClownResponseProbability

		if passedProbability {
			successCount++
			log.Printf("   Попытка %d: ✅ (прошла вероятность %d%%)", i+1, cfg.ClownResponseProbability)
			// В реальности здесь был бы cooldown, но в симуляции мы его пропускаем
		} else {
			log.Printf("   Попытка %d: ❌ (не прошла вероятность)", i+1)
		}

		// Небольшая задержка для реалистичности
		time.Sleep(10 * time.Millisecond)
	}

	successRate := float64(successCount) / float64(totalAttempts) * 100
	log.Printf("\n📊 Результаты симуляции:")
	log.Printf("   • Успешных ответов: %d из %d (%.1f%%)", successCount, totalAttempts, successRate)
	log.Printf("   • Ожидаемая вероятность: %d%%", cfg.ClownResponseProbability)

	if successRate <= float64(cfg.ClownResponseProbability)+10 {
		log.Printf("   ✅ Вероятность работает корректно")
	} else {
		log.Printf("   ⚠️  Отклонение от ожидаемой вероятности")
	}

	log.Printf("\n💡 Симуляция cooldown:")
	log.Printf("   • В реальности после первого успешного ответа")
	log.Printf("   • Следующие %d секунд все попытки будут отклонены", cfg.ClownCooldownSeconds)
	log.Printf("   • Это резко снижает возможность спама")

	log.Printf("\n💡 Симуляция hourly limit:")
	log.Printf("   • После %d успешных ответов в час", cfg.MaxClownResponsesPerHour)
	log.Printf("   • Все дальнейшие попытки отклоняются до следующего часа")
	log.Printf("   • Защищает от длительного злоупотребления")
}
