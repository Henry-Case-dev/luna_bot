package bot

import (
	"log"
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

// TestImageGenerationLimits тестирует лимиты генерации изображений
func TestImageGenerationLimits(t *testing.T) {
	log.Println("=== Тест лимитов генерации изображений Free Will ===")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	log.Printf("✓ Конфигурация загружена")
	log.Printf("✓ Лимиты изображений: макс решений=%d, интервал=%d ч, мин интервал=%d мин",
		cfg.FreeWillImageGenerationMaxDecisionsPerInterval,
		cfg.FreeWillImageGenerationIntervalHours,
		cfg.FreeWillImageGenerationMinDecisionIntervalMinutes)

	// Создаем минимальный Bot для теста (с пустым storage для избежания ошибок)
	testBot := &Bot{
		config:  cfg,
		storage: nil, // Оставляем nil для упрощения теста
	}

	// Создаем Free Will Service
	freeWillService := NewFreeWillService(testBot)

	log.Printf("\\n=== Тестируем лимиты генерации изображений ===")

	// Тест 1: Проверяем минимальный интервал между решениями (основная проверка)
	log.Println("\\n📋 Тест 1: Минимальный интервал между решениями")
	testChatID1 := int64(-1001234567890)
	errors1 := 0

	// Первое решение должно проходить
	if freeWillService.canGenerateImage(testChatID1) {
		freeWillService.updateImageGenerationStats(testChatID1)
		log.Printf("  ✓ Первое решение принято")
	} else {
		log.Printf("  ❌ ОШИБКА: Первое решение должно было быть принято")
		errors1++
		t.Errorf("Первое решение должно было быть принято")
	}

	// Второе решение сразу после первого должно блокироваться (минимальный интервал 30 мин)
	if !freeWillService.canGenerateImage(testChatID1) {
		log.Printf("  ✓ Второе решение сразу после первого корректно заблокировано (интервал 30 мин)")
	} else {
		log.Printf("  ❌ ОШИБКА: Второе решение не должно было быть принято сразу")
		errors1++
		t.Errorf("Второе решение не должно было быть принято сразу")
	}

	if errors1 == 0 {
		log.Printf("  ✅ Тест минимального интервала ПРОЙДЕН")
	} else {
		log.Printf("  ❌ Тест минимального интервала ПРОВАЛЕН с %d ошибками", errors1)
	}

	// Тест 2: Проверяем лимит по количеству решений (симуляция с обходом минимального интервала)
	log.Println("\\n📋 Тест 2: Лимит по количеству решений за интервал")
	testChatID2 := int64(-1001234567891)
	errors2 := 0
	maxDecisions := cfg.FreeWillImageGenerationMaxDecisionsPerInterval

	log.Printf("  Симулируем %d быстрых решений для проверки лимита количества", maxDecisions+2)

	// Получаем доступ к статистике чтобы имитировать принятие решений без минимального интервала
	stats2 := freeWillService.getOrCreateStats(testChatID2)

	// Принимаем решения вручную до лимита
	successfulDecisions := 0
	for i := 0; i < maxDecisions; i++ {
		stats2.ImageGenerationDecisionsThisInterval++
		successfulDecisions++
		log.Printf("    Решение %d/%d принято (симуляция)", successfulDecisions, maxDecisions)
	}

	// Теперь проверяем что превышение лимита блокируется
	if !freeWillService.canGenerateImage(testChatID2) {
		log.Printf("  ✓ Лимит по количеству корректно блокирует дополнительные решения (%d/%d)",
			stats2.ImageGenerationDecisionsThisInterval, maxDecisions)
	} else {
		log.Printf("  ❌ ОШИБКА: Лимит по количеству не сработал")
		errors2++
		t.Errorf("Лимит по количеству должен блокировать дополнительные решения")
	}

	if errors2 == 0 {
		log.Printf("  ✅ Тест лимита по количеству ПРОЙДЕН")
	} else {
		log.Printf("  ❌ Тест лимита по количеству ПРОВАЛЕН с %d ошибками", errors2)
	} // Тест 3: Проверяем конфигурацию
	log.Println("\\n📋 Тест 3: Проверка конфигурации")

	if cfg.FreeWillImageGenerationIndependentLimits {
		log.Printf("  ✓ Независимые лимиты включены в конфигурации")
		log.Printf("  💡 Лимиты изображений работают отдельно от лимитов текстовых ответов")
	} else {
		log.Printf("  ⚠️  Независимые лимиты отключены - изображения используют общие лимиты")
	}

	log.Printf("  🔧 Максимальное количество решений за интервал: %d", cfg.FreeWillImageGenerationMaxDecisionsPerInterval)
	log.Printf("  🔧 Интервал сброса лимитов: %d часов", cfg.FreeWillImageGenerationIntervalHours)
	log.Printf("  🔧 Минимальный интервал между решениями: %d минут", cfg.FreeWillImageGenerationMinDecisionIntervalMinutes)

	// Итоговый отчет
	log.Println("\\n=== Итоговый отчет ===")

	// Проверяем статистику через публичные методы
	allStats := freeWillService.GetAllStats()
	for chatID, stats := range allStats {
		log.Printf("📊 Чат %d:", chatID)
		log.Printf("  - Решений об изображениях: %d", stats.ImageGenerationDecisionsThisInterval)
		log.Printf("  - Последнее решение: %v", stats.LastImageGenerationDecisionTime)
		log.Printf("  - Время сброса интервала: %v", stats.ImageGenerationIntervalResetTime)
	}

	totalErrors := errors1 + errors2
	if totalErrors == 0 {
		log.Println("\\n🎉 ВСЕ ТЕСТЫ ПРОЙДЕНЫ УСПЕШНО!")
	} else {
		log.Printf("\\n💥 ТЕСТЫ ПРОВАЛЕНЫ с общим количеством ошибок: %d", totalErrors)
		t.Fatalf("Тесты провалены с %d ошибками", totalErrors)
	}

	log.Println("\\n✅ Тест лимитов генерации изображений завершен")
}
