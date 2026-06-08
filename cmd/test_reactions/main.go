package main

import (
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

func main() {
	log.Println("🧪 Тест системы реакций")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	log.Printf("📊 Настройки реакций:")
	log.Printf("   REACTIONS_ENABLED: %v", cfg.ReactionsEnabled)
	log.Printf("   CLOWN_REACTION_PROMPT: '%s'", cfg.ClownReactionPrompt)
	log.Printf("   Длина ClownReactionPrompt: %d символов", len(cfg.ClownReactionPrompt))
	log.Printf("   REACTION_ANALYSIS_PROMPT: '%s'", cfg.ReactionAnalysisPrompt)
	log.Printf("   Длина ReactionAnalysisPrompt: %d символов", len(cfg.ReactionAnalysisPrompt))

	log.Printf("\n📊 Настройки веб-поиска:")
	log.Printf("   WEB_SEARCH_ENABLED: %v", cfg.WebSearchEnabled)
	log.Printf("   GOOGLE_SEARCH_API_KEY: %s", maskSecret(cfg.GoogleSearchAPIKey))
	log.Printf("   GOOGLE_SEARCH_ENGINE_ID: %s", cfg.GoogleSearchEngineID)
	log.Printf("   WEB_SEARCH_MAX_RESULTS: %d", cfg.WebSearchMaxResults)

	// Проверки
	issues := []string{}

	if !cfg.ReactionsEnabled {
		issues = append(issues, "⚠️  ReactionsEnabled отключен")
	} else {
		log.Println("✅ ReactionsEnabled включен")
	}

	if cfg.ClownReactionPrompt == "" {
		issues = append(issues, "❌ ClownReactionPrompt пустой!")
	} else {
		log.Println("✅ ClownReactionPrompt настроен")
	}

	if cfg.ReactionAnalysisPrompt == "" {
		issues = append(issues, "⚠️  ReactionAnalysisPrompt пустой")
	} else {
		log.Println("✅ ReactionAnalysisPrompt настроен")
	}

	// Проверка веб-поиска
	if cfg.WebSearchEnabled {
		if cfg.GoogleSearchAPIKey == "" {
			issues = append(issues, "⚠️  WebSearchEnabled, но GoogleSearchAPIKey пустой")
		}
		if cfg.GoogleSearchEngineID == "" {
			issues = append(issues, "⚠️  WebSearchEnabled, но GoogleSearchEngineID пустой")
		}
	}

	// Вывод результатов
	if len(issues) > 0 {
		log.Println("\n🔍 Обнаруженные проблемы:")
		for _, issue := range issues {
			log.Println("   " + issue)
		}
	} else {
		log.Println("\n🎉 Все настройки корректны!")
	}

	// Тестируем возможные причины почему реакции не работают
	log.Println("\n🔍 Диагностика возможных проблем:")

	if cfg.ClownReactionPrompt == "" {
		log.Println("   ❌ ОСНОВНАЯ ПРОБЛЕМА: ClownReactionPrompt пустой - бот не будет отвечать на реакции клоуна")
	}

	if !cfg.ReactionsEnabled {
		log.Println("   ❌ ОСНОВНАЯ ПРОБЛЕМА: ReactionsEnabled=false - система реакций отключена")
	}

	log.Println("\n🔧 Рекомендации:")
	log.Println("   1. Проверьте что в env.txt есть CLOWN_REACTION_PROMPT")
	log.Println("   2. Проверьте что REACTIONS_ENABLED=true в env.txt")
	log.Println("   3. Убедитесь что бот имеет права на получение реакций в группах")
	log.Println("   4. Проверьте логи CustomUpdatesPoller на получение реакций")

	log.Println("\n🏁 Тест завершен")
}

func maskSecret(s string) string {
	if len(s) == 0 {
		return "(пустой)"
	}
	if len(s) < 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}
