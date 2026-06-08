package main

import (
	"fmt"
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	fmt.Println("=== Простой тест настроек веб-поиска ===")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	fmt.Printf("📊 Настройки веб-поиска:\n")
	fmt.Printf("   WEB_SEARCH_ENABLED: %v\n", cfg.WebSearchEnabled)

	// Маскируем API ключ для безопасности
	apiKey := cfg.GoogleSearchAPIKey
	if len(apiKey) > 10 {
		apiKey = apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
	}
	fmt.Printf("   GOOGLE_SEARCH_API_KEY: %s (длина: %d)\n", apiKey, len(cfg.GoogleSearchAPIKey))
	fmt.Printf("   GOOGLE_SEARCH_ENGINE_ID: %s\n", cfg.GoogleSearchEngineID)
	fmt.Printf("   WEB_SEARCH_MAX_RESULTS: %d\n", cfg.WebSearchMaxResults)

	// Показываем промпт с ограничением длины
	triggerPrompt := cfg.WebSearchTriggerPrompt
	if len(triggerPrompt) > 100 {
		triggerPrompt = triggerPrompt[:100] + "..."
	}
	fmt.Printf("   WEB_SEARCH_TRIGGER_PROMPT: %s\n", triggerPrompt)

	// Проверка корректности настроек
	fmt.Printf("\n🔍 Анализ настроек:\n")

	if !cfg.WebSearchEnabled {
		fmt.Printf("   ⚠️  Веб-поиск отключен (WEB_SEARCH_ENABLED=false)\n")
	} else {
		fmt.Printf("   ✅ Веб-поиск включен\n")
	}

	if cfg.GoogleSearchAPIKey == "" {
		fmt.Printf("   ❌ Google Search API Key не настроен\n")
	} else if len(cfg.GoogleSearchAPIKey) < 30 {
		fmt.Printf("   ⚠️  Google Search API Key выглядит подозрительно коротким\n")
	} else {
		fmt.Printf("   ✅ Google Search API Key настроен\n")
	}

	if cfg.GoogleSearchEngineID == "" {
		fmt.Printf("   ❌ Google Search Engine ID не настроен\n")
	} else if len(cfg.GoogleSearchEngineID) < 10 {
		fmt.Printf("   ⚠️  Google Search Engine ID выглядит подозрительно коротким\n")
	} else {
		fmt.Printf("   ✅ Google Search Engine ID настроен\n")
	}

	if cfg.WebSearchMaxResults <= 0 {
		fmt.Printf("   ⚠️  Максимальное количество результатов поиска <= 0\n")
	} else if cfg.WebSearchMaxResults > 10 {
		fmt.Printf("   ⚠️  Максимальное количество результатов поиска слишком большое (%d)\n", cfg.WebSearchMaxResults)
	} else {
		fmt.Printf("   ✅ Максимальное количество результатов поиска корректно (%d)\n", cfg.WebSearchMaxResults)
	}

	if cfg.WebSearchTriggerPrompt == "" {
		fmt.Printf("   ⚠️  Промпт для определения необходимости поиска не настроен\n")
		fmt.Printf("       Будет использоваться простая эвристика по ключевым словам\n")
	} else {
		fmt.Printf("   ✅ Промпт для определения необходимости поиска настроен (%d символов)\n", len(cfg.WebSearchTriggerPrompt))
	}

	// Общий вердикт
	fmt.Printf("\n🎯 Общий статус:\n")

	allGood := cfg.WebSearchEnabled &&
		cfg.GoogleSearchAPIKey != "" &&
		cfg.GoogleSearchEngineID != "" &&
		cfg.WebSearchMaxResults > 0 &&
		cfg.WebSearchMaxResults <= 10

	if allGood {
		fmt.Printf("   ✅ Веб-поиск полностью настроен и готов к использованию\n")
	} else if cfg.WebSearchEnabled {
		fmt.Printf("   ⚠️  Веб-поиск включен, но есть проблемы с настройками\n")
	} else {
		fmt.Printf("   ℹ️  Веб-поиск отключен\n")
	}

	fmt.Printf("\n📖 Как работает веб-поиск:\n")
	fmt.Printf("   1. Активируется только для SERIOUS_DIRECT_PROMPT (серьезные запросы)\n")
	fmt.Printf("   2. Использует промпт или ключевые слова для определения необходимости поиска\n")
	fmt.Printf("   3. Выполняет запрос к Google Custom Search API\n")
	fmt.Printf("   4. Добавляет результаты в начало контекста для LLM\n")
	fmt.Printf("   5. LLM отвечает с учетом актуальной информации из интернета\n")

	fmt.Printf("\n=== Тест настроек завершен ===\n")
}
