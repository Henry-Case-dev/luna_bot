package bot

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// CacheEntry представляет запись в кэше
type CacheEntry struct {
	Results   []SearchResult
	Timestamp time.Time
}

// SearchMetrics содержит метрики использования веб-поиска
type SearchMetrics struct {
	TotalSearches     int64     // Общее количество поисков
	CacheHits         int64     // Количество попаданий в кэш
	CacheMisses       int64     // Количество промахов кэша
	APIErrors         int64     // Количество ошибок API
	KeywordTriggers   int64     // Поиски, инициированные ключевыми словами
	LLMTriggers       int64     // Поиски, инициированные LLM
	AverageResultsNum float64   // Среднее количество результатов
	LastResetTime     time.Time // Время последнего сброса метрик
	mutex             sync.RWMutex
}

// WebSearchService предоставляет функциональность веб-поиска
type WebSearchService struct {
	bot        *Bot
	apiKey     string
	engineID   string
	enabled    bool
	maxResults int
	// Кэширование
	cache      map[string]CacheEntry
	cacheMutex sync.RWMutex
	cacheTTL   time.Duration
	// Метрики
	metrics *SearchMetrics
}

// SearchResult представляет результат поиска
type SearchResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

// GoogleSearchResponse представляет ответ от Google Custom Search API
type GoogleSearchResponse struct {
	Items []SearchResult `json:"items"`
}

// NewWebSearchService создает новый экземпляр WebSearchService
func NewWebSearchService(bot *Bot) *WebSearchService {
	ws := &WebSearchService{
		bot:        bot,
		apiKey:     bot.config.GoogleSearchAPIKey,
		engineID:   bot.config.GoogleSearchEngineID,
		enabled:    bot.config.WebSearchEnabled,
		maxResults: bot.config.WebSearchMaxResults,
		cache:      make(map[string]CacheEntry),
		cacheTTL:   bot.config.WebSearchCacheTTL,
		metrics:    &SearchMetrics{},
	}

	// Запускаем очистку кэша в отдельной горутине
	go ws.startCacheCleanup()

	return ws
}

// IsEnabled проверяет, включен ли веб-поиск
func (ws *WebSearchService) IsEnabled() bool {
	return ws.enabled && ws.apiKey != "" && ws.engineID != ""
}

// simpleNeedSearchHeuristics — быстрые эвристики для определения, что нужен веб-поиск
func (ws *WebSearchService) simpleNeedSearchHeuristics(text string) bool {
	if text == "" {
		return false
	}

	t := strings.ToLower(text)

	// Триггеры актуальности/времени/фактчека
	keywords := []string{
		// Время и актуальность
		"новости", "последние", "актуальны", "сегодня", "вчера", "завтра",
		"недавно", "свеж", "текущ", "сейчас", "в данный момент",

		// Экономика и финансы
		"курс", "цена", "стоимость", "биржа", "валюта", "криптовалют",
		"bitcoin", "ethereum", "доллар", "евро", "рубл", "инфляц",

		// События и факты
		"событи", "что происходит", "произошло", "случилось", "факт",
		"новость", "сводка", "обновлен", "анонс", "релиз",

		// Наука и фактчекинг
		"исследован", "открыт", "эксперимент", "научн", "доказатель",
		"аргумент", "гипотез", "теори", "исследовател", "учен",
		"статистик", "данн", "анализ", "результат", "вывод",
		"peer review", "реценз", "публикац", "журнал",

		// Фактчекинг
		"правда ли", "это правда", "фактчек", "проверить", "подтвердить",
		"опроверг", "миф", "дезинформац", "fake news", "фейк",
		"достоверн", "источник", "верифиц", "подлинн",

		// Справочные и численные вопросы
		"когда", "где", "кто", "сколько", "какой", "что такое", "определение",

		// Погода/ЧП
		"погода", "температур", "прогноз", "землетрясен", "ураган", "цунами",

		// Технологии
		"обновлен", "версия", "релиз", "апдейт", "патч", "уязвимость", "хак",
	}
	for _, kw := range keywords {
		if strings.Contains(t, kw) {
			return true
		}
	}

	// Годы/даты как индикатор актуальности
	yearRe := regexp.MustCompile(`\b20[2-9][0-9]\b`) // 2020+
	if yearRe.MatchString(t) {
		return true
	}

	// Вопросительный знак + вопросительное слово
	if strings.Contains(t, "?") {
		qWords := []string{"когда", "где", "сколько", "какой", "кто", "что"}
		for _, q := range qWords {
			if strings.Contains(t, q) {
				return true
			}
		}
	}

	return false
}

// ShouldPerformSmartSearch — энергоэффективная проверка с учетом контекста и запроса
// 1) Быстрые локальные эвристики по query/context
// 2) Если не сработало и задан WEB_SEARCH_TRIGGER_PROMPT — один короткий LLM‑гейтинг
func (ws *WebSearchService) ShouldPerformSmartSearch(contextText, userQuery string) bool {
	if !ws.IsEnabled() {
		return false
	}

	// Эвристики по самому запросу
	if ws.simpleNeedSearchHeuristics(userQuery) {
		ws.recordKeywordTrigger()
		log.Printf("[INFO][WebSearch] WebSearchDecision: yes (heuristic=query); query=%q", strings.TrimSpace(userQuery))
		return true
	}
	// Эвристики по контексту (если запрос слабый)
	if ws.simpleNeedSearchHeuristics(contextText) {
		ws.recordKeywordTrigger()
		log.Printf("[INFO][WebSearch] WebSearchDecision: yes (heuristic=context); query=%q", strings.TrimSpace(userQuery))
		return true
	}

	// Если промпт триггера не задан — на этом останавливаемся (экономим токены)
	if ws.bot.config.WebSearchTriggerPrompt == "" {
		log.Printf("[INFO][WebSearch] WebSearchDecision: no (heuristics=false, no_trigger_prompt); query=%q", strings.TrimSpace(userQuery))
		return false
	}

	// Один короткий LLM-вызов (дешевая модель/температура низкая)
	prompt := ws.bot.enrichPromptWithPersonality(ws.bot.config.WebSearchTriggerPrompt, 0, "web_search_trigger") +
		"\nКонтекст: " + truncateForPrompt(contextText, 600) +
		"\nЗапрос: " + truncateForPrompt(userQuery, 200)
	response, err := ws.bot.llm.GenerateResponseByType(llm.ResponseTypeWebSearch, prompt, "", float32(ws.bot.config.GeminiTemperatureSerious))
	if err != nil {
		log.Printf("[ERROR][WebSearch] Ошибка определения необходимости поиска (smart): %v", err)
		return false
	}
	r := strings.ToLower(strings.TrimSpace(response))
	should := strings.Contains(r, "yes") || strings.Contains(r, "да")
	if should {
		ws.recordLLMTrigger()
	}
	log.Printf("[INFO][WebSearch] WebSearchDecision: %s (llm_gating, raw=%q); query=%q", map[bool]string{true: "yes", false: "no"}[should], truncateForPrompt(r, 80), strings.TrimSpace(userQuery))
	return should
}

// EnhanceContextWithSmartWebSearch — добавляет результаты веб‑поиска при необходимости
// Принимает несколько кандидатов запросов; использует первый, для которого нужен поиск
func (ws *WebSearchService) EnhanceContextWithSmartWebSearch(originalContext string, queryCandidates ...string) string {
	if !ws.IsEnabled() {
		return originalContext
	}
	started := false
	for _, q := range queryCandidates {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if ws.ShouldPerformSmartSearch(originalContext, q) {
			log.Printf("[INFO][WebSearch] WebSearchStart (smart); query=%q", q)
			results := ws.SearchAndFormat(q)
			if results != "" {
				log.Printf("[INFO][WebSearch] WebSearchEnhancedContext (smart); query=%q", q)
				return results + "\n" + strings.Repeat("-", 50) + "\n\n" + originalContext
			}
			log.Printf("[INFO][WebSearch] WebSearchSkip (smart, no_results); query=%q", q)
			started = true
		}
	}
	if !started {
		log.Printf("[INFO][WebSearch] WebSearchSkip (smart, no_candidate_triggered)")
	}
	return originalContext
}

// truncateForPrompt ограничивает длину строки для безопасного включения в промпт
func truncateForPrompt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ShouldPerformSearch определяет, нужен ли веб-поиск для данного запроса
func (ws *WebSearchService) ShouldPerformSearch(query string) bool {
	if !ws.IsEnabled() {
		return false
	}

	if ws.bot.config.WebSearchTriggerPrompt == "" {
		// Расширенная эвристика: ключевые слова для поиска актуальной информации
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

			// Личности / сленг / тренды
			"кто такой", "кто такая", "что за", "мем", "слэнг", "биография", "персонаж",
			"стример", "ютубер", "инфлюенсер", "артист", "актёр", "актер", "певец", "певица",
			"тренд", "скандал", "турнир", "матч", "лига", "сезон", "серия", "серил", "сериал",
			"фильм", "премьера", "кассовые", "бокс-офис",
		}

		queryLower := strings.ToLower(query)
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				if ws.bot.config.Debug {
					log.Printf("[DEBUG][WebSearch] Найдено ключевое слово '%s' в запросе: %s", keyword, query)
				}
				ws.recordKeywordTrigger()
				log.Printf("[INFO][WebSearch] WebSearchDecision: yes (heuristic=keyword:%s); query=%q", keyword, strings.TrimSpace(query))
				return true
			}
		}
		log.Printf("[INFO][WebSearch] WebSearchDecision: no (heuristics=false); query=%q", strings.TrimSpace(query))
		return false
	}

	// Используем LLM для определения необходимости поиска
	prompt := ws.bot.enrichPromptWithPersonality(ws.bot.config.WebSearchTriggerPrompt, 0, "web_search_trigger") + " " + query
	response, err := ws.bot.llm.GenerateResponseByType(llm.ResponseTypeWebSearch, prompt, "", float32(ws.bot.config.GeminiTemperatureSerious))
	if err != nil {
		log.Printf("[ERROR][WebSearch] Ошибка определения необходимости поиска: %v", err)
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	shouldSearch := strings.Contains(response, "yes") || strings.Contains(response, "да")

	if shouldSearch {
		ws.recordLLMTrigger()
		if ws.bot.config.Debug {
			log.Printf("[DEBUG][WebSearch] LLM рекомендует поиск для запроса: %s", query)
		}
	}

	log.Printf("[INFO][WebSearch] WebSearchDecision: %s (llm_gating, raw=%q); query=%q", map[bool]string{true: "yes", false: "no"}[shouldSearch], truncateForPrompt(response, 80), strings.TrimSpace(query))

	return shouldSearch
}

// Search выполняет веб-поиск и возвращает результаты
func (ws *WebSearchService) Search(query string) ([]SearchResult, error) {
	if !ws.IsEnabled() {
		return nil, fmt.Errorf("веб-поиск отключен или не настроен")
	}

	// Генерируем ключ кэша
	cacheKey := ws.generateCacheKey(query)

	// Проверяем кэш
	if cachedResults, found := ws.getCachedResults(cacheKey); found {
		ws.recordCacheHit()
		if ws.bot.config.Debug {
			log.Printf("[DEBUG][WebSearch] Используем кэшированные результаты для запроса: %s", query)
		}
		return cachedResults, nil
	}

	// Записываем промах кэша
	ws.recordCacheMiss()

	// Подготавливаем URL для Google Custom Search API
	baseURL := "https://www.googleapis.com/customsearch/v1"
	params := url.Values{}
	params.Add("key", ws.apiKey)
	params.Add("cx", ws.engineID)
	params.Add("q", query)
	params.Add("num", fmt.Sprintf("%d", ws.maxResults))
	params.Add("lr", "lang_ru") // Предпочитаем русский язык

	searchURL := baseURL + "?" + params.Encode()

	if ws.bot.config.Debug {
		log.Printf("[DEBUG][WebSearch] Выполняем поиск: %s", query)
	}

	// Выполняем HTTP запрос
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(searchURL)
	if err != nil {
		ws.recordAPIError()
		return nil, fmt.Errorf("ошибка HTTP запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ws.recordAPIError()
		return nil, fmt.Errorf("HTTP ошибка: %d", resp.StatusCode)
	}

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ws.recordAPIError()
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	// Парсим JSON
	var searchResponse GoogleSearchResponse
	err = json.Unmarshal(body, &searchResponse)
	if err != nil {
		ws.recordAPIError()
		return nil, fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	// Записываем успешный результат поиска
	ws.recordSearchResults(len(searchResponse.Items))

	if ws.bot.config.Debug {
		log.Printf("[DEBUG][WebSearch] Найдено результатов: %d", len(searchResponse.Items))
	}

	// Сохраняем результаты в кэше
	ws.setCachedResults(cacheKey, searchResponse.Items)

	return searchResponse.Items, nil
}

// FormatSearchResults форматирует результаты поиска для включения в контекст
func (ws *WebSearchService) FormatSearchResults(results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}

	var formatted strings.Builder
	formatted.WriteString("Актуальная информация из интернета:\n\n")

	for i, result := range results {
		formatted.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
		if result.Snippet != "" {
			// Ограничиваем длину сниппета
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

// SearchAndFormat выполняет поиск и форматирует результаты одним вызовом
func (ws *WebSearchService) SearchAndFormat(query string) string {
	if !ws.ShouldPerformSearch(query) {
		log.Printf("[INFO][WebSearch] WebSearchSkip (decision=no); query=%q", strings.TrimSpace(query))
		return ""
	}

	log.Printf("[INFO][WebSearch] WebSearchStart; query=%q", strings.TrimSpace(query))
	results, err := ws.Search(query)
	if err != nil {
		log.Printf("[ERROR][WebSearch] Ошибка поиска: %v", err)
		return ""
	}
	log.Printf("[INFO][WebSearch] WebSearchResults: count=%d; query=%q", len(results), strings.TrimSpace(query))

	formatted := ws.FormatSearchResults(results)
	if formatted == "" {
		log.Printf("[INFO][WebSearch] WebSearchSkip (no_results_after_format); query=%q", strings.TrimSpace(query))
	}
	return formatted
}

// EnhanceContextWithWebSearch добавляет результаты веб-поиска в контекст
func (ws *WebSearchService) EnhanceContextWithWebSearch(originalContext, userQuery string) string {
	log.Printf("[INFO][WebSearch] WebSearchStart (simple); query=%q", strings.TrimSpace(userQuery))
	searchResults := ws.SearchAndFormat(userQuery)
	if searchResults == "" {
		log.Printf("[INFO][WebSearch] WebSearchSkip (simple, no_results_or_denied); query=%q", strings.TrimSpace(userQuery))
		return originalContext
	}

	// Добавляем результаты поиска в начало контекста
	enhanced := searchResults + "\n" + strings.Repeat("-", 50) + "\n\n" + originalContext

	log.Printf("[INFO][WebSearch] WebSearchEnhancedContext (simple); query=%q", strings.TrimSpace(userQuery))

	return enhanced
}

// generateCacheKey создает ключ кэша для запроса
func (ws *WebSearchService) generateCacheKey(query string) string {
	// Нормализуем запрос для кэширования
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	hash := md5.Sum([]byte(normalizedQuery))
	return hex.EncodeToString(hash[:])
}

// getCachedResults возвращает кэшированные результаты если они есть и не устарели
func (ws *WebSearchService) getCachedResults(cacheKey string) ([]SearchResult, bool) {
	ws.cacheMutex.RLock()
	defer ws.cacheMutex.RUnlock()

	if entry, exists := ws.cache[cacheKey]; exists {
		if time.Since(entry.Timestamp) < ws.cacheTTL {
			if ws.bot.config.Debug {
				log.Printf("[DEBUG][WebSearch] Найдены кэшированные результаты для ключа: %s", cacheKey)
			}
			return entry.Results, true
		}
	}
	return nil, false
}

// setCachedResults сохраняет результаты в кэше
func (ws *WebSearchService) setCachedResults(cacheKey string, results []SearchResult) {
	ws.cacheMutex.Lock()
	defer ws.cacheMutex.Unlock()

	// Проверяем лимит размера кэша
	if len(ws.cache) >= ws.bot.config.WebSearchCacheMaxSize {
		// Удаляем самые старые записи
		ws.cleanOldestEntries(1)
	}

	ws.cache[cacheKey] = CacheEntry{
		Results:   results,
		Timestamp: time.Now(),
	}

	if ws.bot.config.Debug {
		log.Printf("[DEBUG][WebSearch] Результаты сохранены в кэше для ключа: %s", cacheKey)
	}
}

// cleanOldestEntries удаляет указанное количество самых старых записей из кэша
func (ws *WebSearchService) cleanOldestEntries(count int) {
	if len(ws.cache) == 0 {
		return
	}

	// Создаем срез с ключами и временными метками
	type keyTime struct {
		key       string
		timestamp time.Time
	}

	var entries []keyTime
	for key, entry := range ws.cache {
		entries = append(entries, keyTime{key: key, timestamp: entry.Timestamp})
	}

	// Сортируем по времени (самые старые первыми)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].timestamp.After(entries[j].timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Удаляем самые старые записи
	for i := 0; i < count && i < len(entries); i++ {
		delete(ws.cache, entries[i].key)
	}
}

// startCacheCleanup запускает периодическую очистку устаревших записей кэша
func (ws *WebSearchService) startCacheCleanup() {
	ticker := time.NewTicker(ws.cacheTTL / 2) // Очищаем каждые половину TTL
	defer ticker.Stop()

	for range ticker.C {
		ws.cleanExpiredEntries()
	}
}

// cleanExpiredEntries удаляет устаревшие записи из кэша
func (ws *WebSearchService) cleanExpiredEntries() {
	ws.cacheMutex.Lock()
	defer ws.cacheMutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, entry := range ws.cache {
		if now.Sub(entry.Timestamp) > ws.cacheTTL {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(ws.cache, key)
	}

	if ws.bot.config.Debug && len(expiredKeys) > 0 {
		log.Printf("[DEBUG][WebSearch] Удалено %d устаревших записей из кэша", len(expiredKeys))
	}
}

// GetCacheStats возвращает статистику кэша
func (ws *WebSearchService) GetCacheStats() (size int, maxSize int, ttl time.Duration) {
	ws.cacheMutex.RLock()
	defer ws.cacheMutex.RUnlock()

	return len(ws.cache), ws.bot.config.WebSearchCacheMaxSize, ws.cacheTTL
}

// --- Методы для работы с метриками ---

// recordCacheHit записывает попадание в кэш
func (ws *WebSearchService) recordCacheHit() {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()
	ws.metrics.CacheHits++
}

// recordCacheMiss записывает промах кэша
func (ws *WebSearchService) recordCacheMiss() {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()
	ws.metrics.CacheMisses++
}

// recordAPIError записывает ошибку API
func (ws *WebSearchService) recordAPIError() {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()
	ws.metrics.APIErrors++
}

// recordKeywordTrigger записывает поиск по ключевым словам
func (ws *WebSearchService) recordKeywordTrigger() {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()
	ws.metrics.KeywordTriggers++
}

// recordLLMTrigger записывает поиск по рекомендации LLM
func (ws *WebSearchService) recordLLMTrigger() {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()
	ws.metrics.LLMTriggers++
}

// recordSearchResults записывает количество результатов поиска
func (ws *WebSearchService) recordSearchResults(count int) {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()
	ws.metrics.TotalSearches++

	// Обновляем среднее количество результатов
	if ws.metrics.TotalSearches == 1 {
		ws.metrics.AverageResultsNum = float64(count)
	} else {
		// Обновляем среднее с учетом нового значения
		oldAvg := ws.metrics.AverageResultsNum
		ws.metrics.AverageResultsNum = ((oldAvg * float64(ws.metrics.TotalSearches-1)) + float64(count)) / float64(ws.metrics.TotalSearches)
	}
}

// GetMetrics возвращает копию текущих метрик
func (ws *WebSearchService) GetMetrics() SearchMetrics {
	ws.metrics.mutex.RLock()
	defer ws.metrics.mutex.RUnlock()

	// Возвращаем копию для thread-safety
	return SearchMetrics{
		TotalSearches:     ws.metrics.TotalSearches,
		CacheHits:         ws.metrics.CacheHits,
		CacheMisses:       ws.metrics.CacheMisses,
		APIErrors:         ws.metrics.APIErrors,
		KeywordTriggers:   ws.metrics.KeywordTriggers,
		LLMTriggers:       ws.metrics.LLMTriggers,
		AverageResultsNum: ws.metrics.AverageResultsNum,
		LastResetTime:     ws.metrics.LastResetTime,
	}
}

// ResetMetrics сбрасывает все метрики
func (ws *WebSearchService) ResetMetrics() {
	ws.metrics.mutex.Lock()
	defer ws.metrics.mutex.Unlock()

	ws.metrics.TotalSearches = 0
	ws.metrics.CacheHits = 0
	ws.metrics.CacheMisses = 0
	ws.metrics.APIErrors = 0
	ws.metrics.KeywordTriggers = 0
	ws.metrics.LLMTriggers = 0
	ws.metrics.AverageResultsNum = 0
	ws.metrics.LastResetTime = time.Now()
}

// GetCacheHitRate возвращает процент попаданий в кэш
func (ws *WebSearchService) GetCacheHitRate() float64 {
	ws.metrics.mutex.RLock()
	defer ws.metrics.mutex.RUnlock()

	total := ws.metrics.CacheHits + ws.metrics.CacheMisses
	if total == 0 {
		return 0
	}
	return (float64(ws.metrics.CacheHits) / float64(total)) * 100
}

// LogMetrics выводит текущие метрики в лог
func (ws *WebSearchService) LogMetrics() {
	metrics := ws.GetMetrics()
	cacheHitRate := ws.GetCacheHitRate()
	cacheSize, maxCacheSize, ttl := ws.GetCacheStats()

	log.Printf("[INFO][WebSearch] Статистика веб-поиска:")
	log.Printf("   Всего поисков: %d", metrics.TotalSearches)
	log.Printf("   Кэш: попадания=%d, промахи=%d, процент попаданий=%.1f%%",
		metrics.CacheHits, metrics.CacheMisses, cacheHitRate)
	log.Printf("   Кэш: размер=%d/%d, TTL=%v", cacheSize, maxCacheSize, ttl)
	log.Printf("   Триггеры: ключевые слова=%d, LLM=%d",
		metrics.KeywordTriggers, metrics.LLMTriggers)
	log.Printf("   Ошибки API: %d", metrics.APIErrors)
	log.Printf("   Среднее количество результатов: %.1f", metrics.AverageResultsNum)
}
