package main

import (
	"context"
	"fmt"
	"log"

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

	// Создаем бота
	botInstance, err := bot.New(cfg, nil)
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	// Тестируем веб-поиск
	fmt.Println("=== Тест веб-поиска ===")
	fmt.Printf("WebSearchEnabled: %v\n", cfg.WebSearchEnabled)
	fmt.Printf("GoogleSearchAPIKey: %s (len %d)\n", cfg.GoogleSearchAPIKey[:min(10, len(cfg.GoogleSearchAPIKey))]+"...", len(cfg.GoogleSearchAPIKey))
	fmt.Printf("GoogleSearchEngineID: %s\n", cfg.GoogleSearchEngineID)
	fmt.Printf("WebSearchMaxResults: %d\n", cfg.WebSearchMaxResults)
	fmt.Printf("WebSearchTriggerPrompt: %s\n", cfg.WebSearchTriggerPrompt)

	// Получаем веб-поиск сервис
	webSearch := botInstance.GetWebSearchService()

	fmt.Printf("\nIsEnabled: %v\n", webSearch.IsEnabled())

	// Тестируем разные запросы
	queries := []string{
		"Что такое RAG и как он работает",
		"Что такое RAG и как он работает? используй источники в интернете",
		"Какая сегодня погода в Москве",
		"Курс доллара сегодня",
		"Последние новости о нейросетях",
	}

	for _, query := range queries {
		fmt.Printf("\n--- Тестируем запрос: %s ---\n", query)

		shouldSearch := webSearch.ShouldPerformSearch(query)
		fmt.Printf("ShouldPerformSearch: %v\n", shouldSearch)

		if shouldSearch {
			fmt.Println("Выполняем поиск...")
			results, err := webSearch.Search(query)
			if err != nil {
				fmt.Printf("Ошибка поиска: %v\n", err)
			} else {
				fmt.Printf("Найдено результатов: %d\n", len(results))
				for i, result := range results {
					fmt.Printf("%d. %s\n", i+1, result.Title)
					if len(result.Snippet) > 100 {
						fmt.Printf("   %s...\n", result.Snippet[:100])
					} else {
						fmt.Printf("   %s\n", result.Snippet)
					}
					fmt.Printf("   %s\n", result.Link)
				}
			}
		}
	}

	fmt.Println("\n=== Тест завершен ===")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
