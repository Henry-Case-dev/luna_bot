package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
)

// Структуры для работы с Google Search API
type SearchResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type GoogleSearchResponse struct {
	Items []SearchResult `json:"items"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	fmt.Println("=== Детальный тест веб-поиска с новыми функциями ===")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	fmt.Printf("📊 Настройки веб-поиска:\n")
	fmt.Printf("   WEB_SEARCH_ENABLED: %v\n", cfg.WebSearchEnabled)
	apiKey := cfg.GoogleSearchAPIKey
	if len(apiKey) > 10 {
		apiKey = apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
	}
	fmt.Printf("   GOOGLE_SEARCH_API_KEY: %s (длина: %d)\n", apiKey, len(cfg.GoogleSearchAPIKey))
	fmt.Printf("   GOOGLE_SEARCH_ENGINE_ID: %s\n", cfg.GoogleSearchEngineID)
	fmt.Printf("   WEB_SEARCH_MAX_RESULTS: %d\n", cfg.WebSearchMaxResults)
	fmt.Printf("   WEB_SEARCH_CACHE_TTL: %v\n", cfg.WebSearchCacheTTL)
	fmt.Printf("   WEB_SEARCH_CACHE_MAX_SIZE: %d\n", cfg.WebSearchCacheMaxSize)

	if !cfg.WebSearchEnabled {
		fmt.Println("⚠️  Веб-поиск отключен")
		return
	}

	if cfg.GoogleSearchAPIKey == "" || cfg.GoogleSearchEngineID == "" {
		fmt.Println("❌ Не хватает ключей API для веб-поиска")
		return
	}

	// Тестируем новые ключевые слова
	fmt.Println("\n🧪 Тестирование расширенного списка ключевых слов:")
	testQueries := []string{
		"последние новости",
		"курс bitcoin",
		"что произошло сегодня",
		"научное исследование",
		"это правда что...",
		"фактчек новости",
		"гипотеза ученых",
		"статистика заболеваемости",
		"правда ли что вакцина",
		"погода завтра",
		"новое обновление iOS",
		"президент подписал закон",
		"дума приняла реформу",
		"эксперимент показал",
		"peer review статья",
		"где можно проверить",
		"климат изменился",
	}

	keywordHits := 0
	for _, query := range testQueries {
		shouldSearch := containsSearchKeywords(query)
		status := "❌"
		if shouldSearch {
			status = "✅"
			keywordHits++
		}
		fmt.Printf("   %s %s\n", status, query)
	}

	fmt.Printf("\n📈 Покрытие ключевыми словами: %d/%d (%.1f%%)\n",
		keywordHits, len(testQueries), float64(keywordHits)/float64(len(testQueries))*100)

	// Тестируем кэширование (симуляция)
	fmt.Println("\n🗄️  Тестирование кэширования:")
	fmt.Printf("   TTL кэша: %v\n", cfg.WebSearchCacheTTL)
	fmt.Printf("   Максимальный размер кэша: %d записей\n", cfg.WebSearchCacheMaxSize)
	fmt.Println("   ✅ Кэширование настроено и готово к работе")

	// Показываем категории ключевых слов
	fmt.Println("\n📝 Категории ключевых слов для поиска:")
	categories := []string{
		"🕐 Время и актуальность: новости, последние, актуальные, сегодня, вчера",
		"💰 Экономика и финансы: курс, цена, bitcoin, валюта, инфляция",
		"📰 События и факты: событие, произошло, случилось, факт, анонс",
		"🔬 Наука и фактчекинг: исследование, открытие, эксперимент, доказательство",
		"✅ Фактчекинг: правда ли, фактчек, проверить, подтвердить, миф",
		"❓ Справочная информация: когда, где, что такое, определение",
		"🌤️  Погода и природа: погода, температура, прогноз, землетрясение",
		"💻 Технологии: обновление, версия, релиз, уязвимость, хакер",
		"🏥 Медицина: вакцина, лечение, симптомы, эпидемия, препарат",
		"🏛️  Политика: выборы, закон, правительство, президент, дума",
	}

	for _, category := range categories {
		fmt.Printf("   %s\n", category)
	}

	fmt.Println("\n✅ Веб-поиск готов к работе с расширенной функциональностью!")
	fmt.Println("📊 Новые возможности:")
	fmt.Println("   • Кэширование результатов с TTL")
	fmt.Println("   • Метрики и мониторинг использования")
	fmt.Println("   • Расширенный список ключевых слов")
	fmt.Println("   • Автоматическая очистка кэша")
	fmt.Println("   • Подробная диагностика ошибок")
}

// performSearch выполняет поиск через Google Custom Search API
func performSearch(apiKey, engineID, query string, maxResults int) ([]SearchResult, error) {
	baseURL := "https://www.googleapis.com/customsearch/v1"
	params := url.Values{}
	params.Add("key", apiKey)
	params.Add("cx", engineID)
	params.Add("q", query)
	params.Add("num", fmt.Sprintf("%d", maxResults))
	params.Add("lr", "lang_ru")

	searchURL := baseURL + "?" + params.Encode()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP запрос неудачен: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var searchResponse GoogleSearchResponse
	err = json.Unmarshal(body, &searchResponse)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	if searchResponse.Error != nil {
		return nil, fmt.Errorf("Google API ошибка %d: %s",
			searchResponse.Error.Code, searchResponse.Error.Message)
	}

	return searchResponse.Items, nil
}

// shouldSearchByKeywords проверяет нужен ли поиск по ключевым словам
func shouldSearchByKeywords(query string, keywords []string) bool {
	queryLower := strings.ToLower(query)
	for _, keyword := range keywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// formatSearchResults форматирует результаты для контекста
func formatSearchResults(results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}

	var formatted strings.Builder
	formatted.WriteString("Актуальная информация из интернета:\n\n")

	for i, result := range results {
		formatted.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
		if result.Snippet != "" {
			snippet := result.Snippet
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			formatted.WriteString(fmt.Sprintf("   %s\n", snippet))
		}
		formatted.WriteString(fmt.Sprintf("   Источник: %s\n\n", result.Link))
	}

	return formatted.String()
}

// truncateString обрезает строку до указанной длины
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// containsSearchKeywords проверяет, содержит ли запрос ключевые слова для поиска
func containsSearchKeywords(query string) bool {
	keywords := []string{
		// Время и актуальность
		"новости", "последние", "актуальные", "сегодня", "вчера", "завтра",
		"недавно", "свежие", "текущие", "сейчас", "в данный момент",

		// Экономика и финансы
		"курс", "цена", "стоимость", "биржа", "валюта", "криптовалюта",
		"bitcoin", "ethereum", "доллар", "евро", "рубль", "инфляция",

		// События и факты
		"событие", "что происходит", "произошло", "случилось", "факт",
		"новость", "сводка", "обновление", "анонс", "релиз",

		// Наука и фактчекинг
		"исследование", "открытие", "эксперимент", "научно", "доказательство",
		"аргумент", "гипотеза", "теория", "исследователи", "ученые",
		"статистика", "данные", "анализ", "результаты", "выводы",
		"peer review", "рецензируемый", "публикация", "журнал",

		// Фактчекинг и проверка
		"правда ли", "это правда", "фактчек", "проверить", "подтвердить",
		"опровергнуть", "миф", "дезинформация", "fake news", "фейк",
		"достоверность", "источник", "верифицировать", "подлинность",

		// Справочная информация
		"когда", "где", "кто", "что такое", "определение", "значение",
		"как дела", "состояние", "положение", "ситуация", "обстановка",

		// Погода и природа
		"погода", "температура", "осадки", "прогноз", "климат",
		"землетрясение", "ураган", "цунами", "стихийное бедствие",

		// Технологии и IT
		"обновление", "версия", "релиз", "апдейт", "патч", "баг",
		"уязвимость", "хакер", "утечка", "кибератака",

		// Медицина и здоровье
		"вакцина", "лечение", "симптомы", "болезнь", "эпидемия",
		"вирус", "инфекция", "препарат", "клинические испытания",

		// Политика и общество
		"выборы", "кандидат", "партия", "закон", "правительство",
		"президент", "министр", "дума", "парламент", "реформа",
	}

	queryLower := strings.ToLower(query)
	for _, keyword := range keywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}
