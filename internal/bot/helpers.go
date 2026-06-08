package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	"github.com/Henry-Case-dev/luna_bot/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// formatRemainingTime форматирует оставшееся время
func formatRemainingTime(d time.Duration) string {
	if d <= 0 {
		return "0с"
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	parts := []string{}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dч", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dм", minutes))
	}
	if seconds > 0 || len(parts) == 0 { // Показываем секунды, если нет часов/минут, или если время < 1 минуты
		parts = append(parts, fmt.Sprintf("%dс", seconds))
	}

	return strings.Join(parts, " ")
}

// saveChatSettings сохраняет настройки чата в JSON файл
func saveChatSettings(chatID int64, settings *ChatSettings) error {
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("ошибка создания директории data: %w", err)
	}

	filename := filepath.Join(dataDir, fmt.Sprintf("chat_%d_settings.json", chatID))
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("ошибка создания файла настроек %s: %w", filename, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Для читаемости файла
	if err := encoder.Encode(settings); err != nil {
		return fmt.Errorf("ошибка кодирования настроек в JSON для чата %d: %w", chatID, err)
	}
	return nil
}

// isAdmin проверяет, является ли пользователь администратором бота
func (b *Bot) isAdmin(user *tgbotapi.User) bool {
	if user == nil {
		return false
	}

	// Проверяем по ID (основной способ)
	if b.config.AdminID != 0 && user.ID == b.config.AdminID {
		return true
	}

	// Проверяем по username (резервный способ)
	for _, adminUsername := range b.config.AdminUsernames {
		if strings.EqualFold(user.UserName, adminUsername) {
			return true
		}
	}
	return false
}

// getUserIDByUsername ищет ID пользователя по его @username в профилях чата
func (b *Bot) getUserIDByUsername(chatID int64, username string) (int64, error) {
	profiles, err := b.storage.GetAllUserProfiles(chatID)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения профилей: %w", err)
	}

	cleanUsername := strings.TrimPrefix(username, "@")

	for _, p := range profiles {
		if strings.EqualFold(p.Username, cleanUsername) {
			return p.UserID, nil
		}
	}

	return 0, fmt.Errorf("пользователь @%s не найден в профилях этого чата", cleanUsername)
}

// findUserProfileByUsername ищет профиль пользователя по его @username
func (b *Bot) findUserProfileByUsername(chatID int64, username string) (*storage.UserProfile, error) {
	profiles, err := b.storage.GetAllUserProfiles(chatID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения профилей: %w", err)
	}

	cleanUsername := strings.TrimPrefix(username, "@")

	for _, p := range profiles {
		if strings.EqualFold(p.Username, cleanUsername) {
			return p, nil
		}
	}

	return nil, fmt.Errorf("профиль пользователя @%s не найден", cleanUsername)
}

// formatHistoryWithProfiles форматирует историю сообщений для передачи в LLM
// с добавлением информации о профилях пользователей и personality memory
func formatHistoryWithProfiles(chatID int64, messages []*tgbotapi.Message, store storage.ChatHistoryStorage, cfg *config.Config, timeZone string) string {
	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(store, timeZone)
	return formatter.FormatMessages(chatID, messages)
}

// formatDirectReplyContext создает контекст для прямого ответа бота
func formatDirectReplyContext(chatID int64,
	triggeringMessage *tgbotapi.Message, // Сообщение, вызвавшее ответ
	replyChain []*tgbotapi.Message,
	commonContext []*tgbotapi.Message,
	relevantMessages []*tgbotapi.Message,
	store storage.ChatHistoryStorage,
	cfg *config.Config,
	timeZone string) string {

	// Определяем тип контекста на основе сценария
	isDirectMention := false
	isReplyToBot := false

	// Проверяем является ли это прямым упоминанием или ответом на бота
	if triggeringMessage != nil {
		// Проверяем упоминание бота в тексте сообщения
		if strings.Contains(strings.ToLower(triggeringMessage.Text), "катя") ||
			strings.Contains(strings.ToLower(triggeringMessage.Text), "luna") {
			isDirectMention = true
		}

		// Проверяем является ли это ответом на сообщение бота
		if triggeringMessage.ReplyToMessage != nil && triggeringMessage.ReplyToMessage.From != nil {
			isReplyToBot = triggeringMessage.ReplyToMessage.From.IsBot
		}
	}

	contextType := determineContextType("direct_response", triggeringMessage, isDirectMention, isReplyToBot)
	contextType = validateContextType(contextType, triggeringMessage, isDirectMention, isReplyToBot)

	log.Printf("[INFO] Chat %d: Определен контекст '%s' для прямого ответа (упоминание: %v, ответ_на_бота: %v)",
		chatID, contextType, isDirectMention, isReplyToBot)

	// Объединяем все сообщения для контекста
	var allMessages []*tgbotapi.Message

	// УЛУЧШЕНИЕ: Валидируем и исправляем цепочку ответов если есть
	if len(replyChain) > 0 {
		validatedChain, warnings := fixReplyChainUserIdentification(chatID, replyChain, store)
		if len(warnings) > 0 {
			for _, warning := range warnings {
				log.Printf("[WARN] Chat %d: Проблема в цепочке ответов: %s", chatID, warning)
			}
		}
		allMessages = append(allMessages, validatedChain...)
	}

	// Добавляем общий контекст (лимитируем до 40 последних сообщений)
	if len(commonContext) > 0 {
		cutoff := 40
		startIdx := len(commonContext)
		if startIdx > cutoff {
			startIdx = startIdx - cutoff
		} else {
			startIdx = 0
		}
		recentContext := commonContext[startIdx:]
		allMessages = append(allMessages, recentContext...)
	}

	// Добавляем сообщение-триггер в конец если оно не дублируется
	if triggeringMessage != nil {
		// Проверяем нет ли уже этого сообщения в контексте
		found := false
		for _, msg := range allMessages {
			if msg.MessageID == triggeringMessage.MessageID {
				found = true
				break
			}
		}
		if !found {
			allMessages = append(allMessages, triggeringMessage)
		}
	}

	// УЛУЧШЕНИЕ: Сортируем все сообщения по времени для обеспечения правильного хронологического порядка
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].Date < allMessages[j].Date
	})

	// Логируем информацию о сортировке
	if len(allMessages) > 1 {
		log.Printf("[INFO] Chat %d: Отсортировано %d сообщений по времени для контекста (от %d до %d)",
			chatID, len(allMessages), allMessages[0].Date, allMessages[len(allMessages)-1].Date)
	}

	// Дополнительная проверка: убеждаемся что сообщение-триггер остается в конце
	if triggeringMessage != nil {
		// Находим индекс сообщения-триггера
		triggerIndex := -1
		for i, msg := range allMessages {
			if msg.MessageID == triggeringMessage.MessageID {
				triggerIndex = i
				break
			}
		}

		// Если сообщение-триггер не последнее по времени, но является триггером - перемещаем в конец
		if triggerIndex != -1 && triggerIndex != len(allMessages)-1 {
			// Удаляем из текущей позиции
			triggerMsg := allMessages[triggerIndex]
			allMessages = append(allMessages[:triggerIndex], allMessages[triggerIndex+1:]...)
			// Добавляем в конец
			allMessages = append(allMessages, triggerMsg)
			log.Printf("[INFO] Chat %d: Сообщение-триггер %d перемещено в конец контекста", chatID, triggerMsg.MessageID)
		}
	}

	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(store, timeZone)
	return formatter.FormatMessages(chatID, allMessages)
}

// --- Глобальный указатель на Bot для доступа к памяти личности ---
var globalBotInstance *Bot

// Функция для генерации "контрастного" контекста (нерелевантных сообщений)
func generateContrastiveContext(mainContext string, messages []*tgbotapi.Message) string {
	// TODO: Реализовать выбор нерелевантных сообщений (например, старые темы, не связанные с текущим диалогом)
	var contrastBuf strings.Builder
	contrastBuf.WriteString("\n--- IRRELEVANT INFORMATION (IGNORE) ---\n")
	// ...
	contrastBuf.WriteString("\n--- END OF IRRELEVANT INFORMATION ---\n")
	return mainContext + contrastBuf.String()
}

// formatMessagesWithProfilesInternal - устаревшая функция, заменена на UnifiedMessageFormatter
// Оставлена для обратной совместимости
func formatMessagesWithProfilesInternal(chatID int64, messages []*tgbotapi.Message, store storage.ChatHistoryStorage, cfg *config.Config, timeZone string, seenMessageIDs map[int]bool) string {
	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(store, timeZone)
	return formatter.FormatMessages(chatID, messages)
}

// cleanupLLMResponse очищает ответ LLM от различных системных элементов и метаданных
func cleanupLLMResponse(originalResponse string) string {
	log.Printf("🧹 [CLEANUP] Начинаем очистку ответа LLM. Исходная длина: %d символов", len(originalResponse))

	cleanedResponse := originalResponse

	// 1. Убираем теги XML/HTML
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	cleanedResponse = htmlTagRegex.ReplaceAllString(cleanedResponse, "")

	// Основная очистка от метаданных времени и ID
	timeIDRegex := regexp.MustCompile(`(?m)^\d{1,2}:\d{2}(:\d{2})?\s*\([^()]*(\s*\(ID:[0-9]+\)|\s*\(id:[0-9]+\))?(\)|:)?\)\s*`)
	cleanedResponse = timeIDRegex.ReplaceAllString(cleanedResponse, "")

	timeWithBracketsRegex := regexp.MustCompile(`(?m)^\[\d{1,2}:\d{2}(:\d{2})?\]\s*[^:]*:\s*`)
	cleanedResponse = timeWithBracketsRegex.ReplaceAllString(cleanedResponse, "")

	timeNameRegex := regexp.MustCompile(`(?m)^\d{1,2}:\d{2}(:\d{2})?\s+[^:]+:\s*`)
	cleanedResponse = timeNameRegex.ReplaceAllString(cleanedResponse, "")

	idLineRegex := regexp.MustCompile(`(?m)^\d{1,2}:\d{2}(:\d{2})?(.*?(ID:|id:)[^\n]*\n)`)
	cleanedResponse = idLineRegex.ReplaceAllString(cleanedResponse, "")

	nameIDRegex := regexp.MustCompile(`(?m)^[^:]+\s*\(ID:[0-9]+\):\s*`)
	cleanedResponse = nameIDRegex.ReplaceAllString(cleanedResponse, "")

	// === НОВАЯ КРИТИЧЕСКАЯ ОЧИСТКА ПРОФИЛЬНОЙ ИНФОРМАЦИИ ===

	// 1. Удаляем утечки Bio информации - в начале и середине текста
	bioRegex := regexp.MustCompile(`(?i)\s*\(.*Bio:\s*[^)]*.*?\)\s*`)
	cleanedResponse = bioRegex.ReplaceAllString(cleanedResponse, " ")

	// 2. Удаляем утечки AutoBio информации
	autoBioRegex := regexp.MustCompile(`(?i)\s*\(.*AutoBio:\s*[^)]*.*?\)\s*`)
	cleanedResponse = autoBioRegex.ReplaceAllString(cleanedResponse, " ")

	// 3. Удаляем конструкции вида "(Bio: ...; AutoBio: ...)"
	combinedProfileRegex := regexp.MustCompile(`(?i)\s*\(\s*(Bio|AutoBio):\s*[^)]*\)\s*`)
	cleanedResponse = combinedProfileRegex.ReplaceAllString(cleanedResponse, " ")

	// 4. Удаляем цитирование UserID напрямую в виде числовых конструкций в скобках после Bio/AutoBio
	userIDRegex := regexp.MustCompile(`(?i)\s*\(\s*ID:\s*\d+\s*\)\s*`)
	cleanedResponse = userIDRegex.ReplaceAllString(cleanedResponse, " ")

	// 5. Удаляем оставшиеся метки времени и даты
	timeStampRegex := regexp.MustCompile(`(?m)^\[\d{2}:\d{2}(:\d{2})?\]\s*`)
	cleanedResponse = timeStampRegex.ReplaceAllString(cleanedResponse, "")

	// 6. Удаляем оставшиеся конструкции с ID
	remainingIDRegex := regexp.MustCompile(`\s*\(ID:\s*\d+\)\s*`)
	cleanedResponse = remainingIDRegex.ReplaceAllString(cleanedResponse, " ")

	// 7. Удаляем возможные утечки контекста
	contextLeakRegex := regexp.MustCompile(`(?i)(?m)^(Контекст|История чата|Сообщения чата|Текущее время|Анализируемый контекст)[^\n]*\n`)
	cleanedResponse = contextLeakRegex.ReplaceAllString(cleanedResponse, "")

	// 8. Удаляем возможные маркеры ролей
	roleMarkersRegex := regexp.MustCompile(`(?i)(?m)^(ТЫ \(УЧАСТНИК\)|СОБЕСЕДНИК)[^\n]*\n`)
	cleanedResponse = roleMarkersRegex.ReplaceAllString(cleanedResponse, "")

	// 9. Удаляем упоминания пользователей в формате @U[digits]
	userMentionRegex := regexp.MustCompile(`@U\d+`)
	cleanedResponse = userMentionRegex.ReplaceAllString(cleanedResponse, "")

	// 10. Удаляем упоминания количества пользователей в скобках вида "(12 пользователей)"
	userCountRegex := regexp.MustCompile(`\s*\(\d+\s+пользовател[ейя]*\)\s*`)
	cleanedResponse = userCountRegex.ReplaceAllString(cleanedResponse, "")

	// 11. Удаляем пустые скобки "()" после имен пользователей
	emptyBracketsRegex := regexp.MustCompile(`\(\)`)
	cleanedResponse = emptyBracketsRegex.ReplaceAllString(cleanedResponse, "")

	// 12. Удаляем множественные пробелы, сохраняя переносы строк
	multipleSpacesRegex := regexp.MustCompile(`[ \t]+`)
	cleanedResponse = multipleSpacesRegex.ReplaceAllString(cleanedResponse, " ")

	// 13. Удаляем пробелы в начале и конце каждой строки
	lines := strings.Split(cleanedResponse, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	cleanedResponse = strings.Join(lines, "\n")

	// 14. Удаляем множественные переносы строк (больше двух)
	multipleNewlinesRegex := regexp.MustCompile(`\n{3,}`)
	cleanedResponse = multipleNewlinesRegex.ReplaceAllString(cleanedResponse, "\n\n")

	// 15. Финальная очистка от пробелов в начале и конце всего текста
	cleanedResponse = strings.TrimSpace(cleanedResponse)

	// 16. Удаляем ВСЕ кавычки из сообщения (LLM злоупотребляет ими)
	// Убираем все типы кавычек: обычные, елочки, английские, немецкие
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `"`, "") // Обычные прямые кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `"`, "") // Левые английские кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `"`, "") // Правые английские кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `«`, "") // Левые елочки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `»`, "") // Правые елочки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `„`, "") // Немецкие нижние кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `'`, "") // Одинарные кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `'`, "") // Левые одинарные кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, `'`, "") // Правые одинарные кавычки
	cleanedResponse = strings.ReplaceAll(cleanedResponse, "`", "") // Обратные кавычки (backticks)

	// 17. Убираем markdown форматирование
	// Удаляем жирный шрифт (**text** или __text__)
	boldRegex := regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	cleanedResponse = boldRegex.ReplaceAllString(cleanedResponse, "$1$2")

	// Удаляем курсив (*text* или _text_) - но аккуратно, чтобы не затронуть обычные символы
	italicRegex := regexp.MustCompile(`\*([^*\s][^*]*[^*\s])\*|_([^_\s][^_]*[^_\s])_`)
	cleanedResponse = italicRegex.ReplaceAllString(cleanedResponse, "$1$2")

	// Удаляем код в обратных кавычках (`code` или ```code```) - уже не актуально т.к. backticks убраны выше
	codeRegex := regexp.MustCompile("```[^`]*```|`[^`]*`")
	cleanedResponse = codeRegex.ReplaceAllStringFunc(cleanedResponse, func(match string) string {
		if strings.HasPrefix(match, "```") {
			return strings.Trim(match, "`")
		}
		return strings.Trim(match, "`")
	})

	// Удаляем зачеркивание (~~text~~)
	strikethroughRegex := regexp.MustCompile(`~~([^~]+)~~`)
	cleanedResponse = strikethroughRegex.ReplaceAllString(cleanedResponse, "$1$2")

	// === НОВАЯ ОЧИСТКА СТРУКТУРИРОВАННЫХ ТЕГОВ СООБЩЕНИЙ ===

	// 18. Удаляем полные блоки структурированных сообщений [MSG_START]...[MSG_END]
	// Это защищает от случаев, когда LLM копирует весь блок сообщения в ответ
	msgBlockRegex := regexp.MustCompile(`(?s)\[MSG_START\].*?\[MSG_END\]\s*`)
	beforeStructCleanup := cleanedResponse
	cleanedResponse = msgBlockRegex.ReplaceAllString(cleanedResponse, "")
	if beforeStructCleanup != cleanedResponse {
		log.Printf("🧹 [CLEANUP] Удалены структурированные блоки MSG_START/MSG_END. Было %d символов, стало %d", len(beforeStructCleanup), len(cleanedResponse))
	}

	// Убираем отдельные структурированные метаданные, если они попали в ответ
	structuredMetadataRegex := regexp.MustCompile(`(?m)^(Время|Дата|Автор|Bio|Тип|Ответ_на_сообщение|Автор_исходного_сообщения|Текст|ID):\s*.*$\s*`)
	beforeMetadataCleanup := cleanedResponse
	cleanedResponse = structuredMetadataRegex.ReplaceAllString(cleanedResponse, "")
	if beforeMetadataCleanup != cleanedResponse {
		log.Printf("🧹 [CLEANUP] Удалены структурированные метаданные. Было %d символов, стало %d", len(beforeMetadataCleanup), len(cleanedResponse))
	}

	// Убираем ссылки на сообщения типа MSG:123 или #123
	msgReferencesRegex := regexp.MustCompile(`(?i)\b(msg|message):\s*\d+\b|#\d+\b`)
	beforeMsgRefsCleanup := cleanedResponse
	cleanedResponse = msgReferencesRegex.ReplaceAllString(cleanedResponse, "")
	if beforeMsgRefsCleanup != cleanedResponse {
		log.Printf("🧹 [CLEANUP] Удалены ссылки на сообщения. Было %d символов, стало %d", len(beforeMsgRefsCleanup), len(cleanedResponse))
	}

	// Убираем метки типа [изображение], [голосовое сообщение] и т.д.
	mediaLabelsRegex := regexp.MustCompile(`\[(голосовое сообщение|изображение|документ|аудио файл|видео|стикер|специальное сообщение)(?:[^\]]*)\]`)
	beforeMediaLabelsCleanup := cleanedResponse
	cleanedResponse = mediaLabelsRegex.ReplaceAllString(cleanedResponse, "")
	if beforeMediaLabelsCleanup != cleanedResponse {
		log.Printf("🧹 [CLEANUP] Удалены метки медиа. Было %d символов, стало %d", len(beforeMediaLabelsCleanup), len(cleanedResponse))
	}

	// Итоговая статистика очистки
	log.Printf("✅ [CLEANUP] Очистка завершена. Исходная длина: %d символов, итоговая: %d символов (удалено: %d)",
		len(originalResponse), len(cleanedResponse), len(originalResponse)-len(cleanedResponse))

	if len(cleanedResponse) > 200 {
		log.Printf("📝 [CLEANUP] Начало очищенного ответа: %.200s...", cleanedResponse)
	} else {
		log.Printf("📝 [CLEANUP] Полный очищенный ответ: %s", cleanedResponse)
	}

	return cleanedResponse
}

// cleanJSONFromMarkdown очищает ответ LLM от markdown разметки и исправляет псевдо-JSON
//
// УСИЛЕНИЯ В ЭТОЙ ВЕРСИИ:
// - Более надежное извлечение JSON из текста с разметкой
// - Поддержка кириллических ключей в псевдо-JSON
// - Умная обработка вложенных структур (массивы, объекты)
// - Исправление конкретных полей для PersonalityAnalysisResult
// - Обработка объектов в значениях (например, temporal_traits)
// - Финальная очистка от лишних запятых
func cleanJSONFromMarkdown(response string) string {
	// Удаляем markdown code blocks
	response = strings.ReplaceAll(response, "```json", "")
	response = strings.ReplaceAll(response, "```JSON", "")
	response = strings.ReplaceAll(response, "```", "")

	// Удаляем все backticks
	response = strings.ReplaceAll(response, "`", "")

	// Удаляем возможные префиксы
	response = strings.TrimPrefix(response, "json")
	response = strings.TrimPrefix(response, "JSON")

	response = strings.TrimSpace(response)

	// КРИТИЧЕСКОЕ ИСПРАВЛЕНИЕ: Извлекаем только JSON, удаляя текст до и после
	startIdx := strings.Index(response, "{")
	if startIdx == -1 {
		// Нет открывающей скобки, возможно это не JSON
		return response
	}

	// Находим соответствующую закрывающую скобку
	braceCount := 0
	endIdx := -1
	for i := startIdx; i < len(response); i++ {
		switch response[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				endIdx = i
				goto foundBrace
			}
		}
	}
foundBrace:

	if endIdx == -1 {
		// Не найдена парная закрывающая скобка
		return response
	}

	// Извлекаем только JSON часть
	jsonPart := response[startIdx : endIdx+1]

	// УСИЛЕННОЕ ИСПРАВЛЕНИЕ: Пробуем исправить псевдо-JSON (ключи без кавычек)
	jsonPart = fixPseudoJSON(jsonPart)

	return jsonPart
}

// fixPseudoJSON пытается исправить псевдо-JSON форматы от LLM
func fixPseudoJSON(input string) string {
	// Проверяем, является ли это псевдо-JSON (содержит ключи без кавычек)
	if !strings.Contains(input, "{") || !strings.Contains(input, "}") {
		return input
	}

	// Пробуем парсить как есть - если получается, возвращаем как есть
	var testJSON map[string]interface{}
	if err := json.Unmarshal([]byte(input), &testJSON); err == nil {
		return input
	}

	log.Printf("[DEBUG] fixPseudoJSON: Исправляем псевдо-JSON: %s", input)

	fixed := input

	// ЭТАП 1: Предварительная очистка
	// Удаляем лишние пробелы и переносы строк
	fixed = regexp.MustCompile(`\s+`).ReplaceAllString(fixed, " ")
	fixed = strings.TrimSpace(fixed)

	// ЭТАП 2: Исправляем ключи без кавычек
	// Более надежный regex для ключей (поддерживает подчеркивания, цифры, кириллицу)
	keyRegex := regexp.MustCompile(`(\s*|\{|,)\s*([a-zA-Zа-яА-Я_][a-zA-Zа-яА-Я0-9_]*)\s*:`)
	fixed = keyRegex.ReplaceAllString(fixed, `$1"$2":`)

	// ЭТАП 3: Исправляем конкретные поля для PersonalityAnalysisResult
	fieldMappings := map[string]string{
		`"selfperceptions"`:       `"self_perceptions"`,
		`"currentviews"`:          `"current_views"`,
		`"temporaltraits"`:        `"temporal_traits"`,
		`"contextualadaptations"`: `"contextual_adaptations"`,
		`"self_perception"`:       `"self_perceptions"`,
		`"current_view"`:          `"current_views"`,
		`"temporal_trait"`:        `"temporal_traits"`,
		`"contextual_adaptation"`: `"contextual_adaptations"`,
	}

	for wrongField, correctField := range fieldMappings {
		fixed = strings.ReplaceAll(fixed, wrongField, correctField)
	}

	// ЭТАП 4: Исправляем массивы - более надежная обработка
	arrayRegex := regexp.MustCompile(`\[([^\]]*)\]`)
	fixed = arrayRegex.ReplaceAllStringFunc(fixed, func(match string) string {
		content := strings.Trim(match, "[]")
		content = strings.TrimSpace(content)

		if content == "" {
			return "[]"
		}

		// Разбиваем по запятым, но учитываем вложенные структуры
		elements := smartSplit(content, ',')
		var fixedElements []string

		for _, elem := range elements {
			elem = strings.TrimSpace(elem)
			if elem == "" {
				continue
			}

			// Проверяем, нужно ли добавлять кавычки
			if shouldQuoteValue(elem) {
				// Экранируем внутренние кавычки
				elem = strings.ReplaceAll(elem, `"`, `\"`)
				elem = `"` + elem + `"`
			}
			fixedElements = append(fixedElements, elem)
		}

		return "[" + strings.Join(fixedElements, ", ") + "]"
	})

	// ЭТАП 5: Исправляем значения объектов без кавычек
	// Более точный regex для значений
	valueRegex := regexp.MustCompile(`"([^"]+)":\s*([^,}\]]+?)(\s*[,}\]])`)
	fixed = valueRegex.ReplaceAllStringFunc(fixed, func(match string) string {
		parts := valueRegex.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}

		key := parts[1]
		value := strings.TrimSpace(parts[2])
		suffix := parts[3]

		// НЕ изменяем если это уже правильно оформлено
		if !shouldQuoteValue(value) {
			return match
		}

		// Экранируем кавычки в значении и добавляем кавычки
		value = strings.ReplaceAll(value, `"`, `\"`)
		return `"` + key + `": "` + value + `"` + suffix
	})

	// ЭТАП 6: Исправляем объекты в значениях (например, temporal_traits)
	objectValueRegex := regexp.MustCompile(`"([^"]+)":\s*\{([^}]*)\}`)
	fixed = objectValueRegex.ReplaceAllStringFunc(fixed, func(match string) string {
		parts := objectValueRegex.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		key := parts[1]
		objectContent := strings.TrimSpace(parts[2])

		if objectContent == "" {
			return `"` + key + `": {}`
		}

		// Исправляем содержимое объекта
		fixedObjectContent := fixObjectContent(objectContent)
		return `"` + key + `": {` + fixedObjectContent + `}`
	})

	// ЭТАП 7: Финальная проверка и очистка
	fixed = strings.ReplaceAll(fixed, ",}", "}")
	fixed = strings.ReplaceAll(fixed, ",]", "]")

	log.Printf("[DEBUG] fixPseudoJSON: Результат исправления: %s", fixed)

	// Проверяем, получился ли валидный JSON
	if err := json.Unmarshal([]byte(fixed), &testJSON); err == nil {
		log.Printf("[DEBUG] fixPseudoJSON: ✅ Успешно исправлен псевдо-JSON")
		return fixed
	} else {
		log.Printf("[DEBUG] fixPseudoJSON: ❌ Не удалось исправить псевдо-JSON, ошибка: %v", err)
	}
	// Если не получилось - возвращаем исходный
	return input
}

// shouldQuoteValue определяет, нужно ли заключать значение в кавычки
func shouldQuoteValue(value string) bool {
	value = strings.TrimSpace(value)

	// НЕ добавляем кавычки если:
	// 1. Уже есть кавычки
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return false
	}

	// 2. Это число
	if isNumber(value) {
		return false
	}

	// 3. Это boolean
	if value == "true" || value == "false" {
		return false
	}

	// 4. Это null
	if value == "null" {
		return false
	}

	// 5. Это объект или массив
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return false
	}

	return true
}

// smartSplit разбивает строку по разделителю, учитывая вложенные структуры
func smartSplit(s string, delimiter rune) []string {
	var result []string
	var current strings.Builder
	var depth int
	var inQuotes bool
	var escapeNext bool

	for _, r := range s {
		if escapeNext {
			current.WriteRune(r)
			escapeNext = false
			continue
		}

		if r == '\\' {
			escapeNext = true
			current.WriteRune(r)
			continue
		}

		if r == '"' {
			inQuotes = !inQuotes
			current.WriteRune(r)
			continue
		}

		if !inQuotes {
			if r == '{' || r == '[' {
				depth++
			} else if r == '}' || r == ']' {
				depth--
			}
		}

		if r == delimiter && depth == 0 && !inQuotes {
			result = append(result, current.String())
			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// fixObjectContent исправляет содержимое объекта (ключи и значения)
func fixObjectContent(content string) string {
	// Разбиваем по запятым
	pairs := smartSplit(content, ',')
	var fixedPairs []string

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		// Ищем разделитель ключ:значение
		colonIdx := strings.Index(pair, ":")
		if colonIdx == -1 {
			fixedPairs = append(fixedPairs, pair)
			continue
		}

		key := strings.TrimSpace(pair[:colonIdx])
		value := strings.TrimSpace(pair[colonIdx+1:])

		// Исправляем ключ (добавляем кавычки если нужно)
		if !strings.HasPrefix(key, `"`) || !strings.HasSuffix(key, `"`) {
			// Убираем существующие кавычки и добавляем правильные
			key = strings.Trim(key, `"`)
			key = `"` + key + `"`
		}

		// Исправляем значение (добавляем кавычки если нужно)
		if shouldQuoteValue(value) {
			value = strings.ReplaceAll(value, `"`, `\"`)
			value = `"` + value + `"`
		}

		fixedPairs = append(fixedPairs, key+": "+value)
	}

	return strings.Join(fixedPairs, ", ")
}

// isNumber проверяет, является ли строка числом
func isNumber(s string) bool {
	// Проверяем целые числа
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}
	// Проверяем числа с плавающей точкой
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

// sanitizeMarkdown проверяет и исправляет возможные проблемы с Markdown разметкой
// Функция обеспечивает, что текст будет корректно отображаться при отправке с ParseMode=Markdown
func sanitizeMarkdown(text string) string {
	if text == "" {
		return ""
	}

	// Для таблиц нам нужен специальный подход
	// В Telegram Markdown V1 таблицы не поддерживаются напрямую,
	// но можно использовать моноширинный текст для правильного отображения
	if strings.Contains(text, "|") && strings.Contains(text, "---") {
		// Проверяем, является ли это таблицей (наличие строк с |)
		lines := strings.Split(text, "\n")
		hasTableStructure := false

		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			// Проверяем строки заголовка и разделителя таблицы
			if (strings.HasPrefix(trimmedLine, "|") && strings.HasSuffix(trimmedLine, "|")) ||
				(strings.Contains(trimmedLine, "|") && strings.Contains(trimmedLine, "---")) {
				hasTableStructure = true
				break
			}
		}

		if hasTableStructure {
			// Выделяем символы разметки Markdown вокруг текста, кроме таблиц
			// Заменяем их на другие символы, чтобы они не конфликтовали с разметкой
			text = strings.ReplaceAll(text, "*", "•")
			text = strings.ReplaceAll(text, "_", "≺")
			text = strings.ReplaceAll(text, "`", "·")

			// Заменяем ссылки, чтобы они не ломали таблицу
			// Находим все ссылки формата [text](url) и заменяем их на "text (url)"
			linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
			text = linkRegex.ReplaceAllString(text, "$1 ($2)")

			return text
		}
	}

	// Для обычного текста без таблиц используем более мягкий подход -
	// не экранируем, так как Markdown V1 менее строгий

	// Обрабатываем специальные случаи для Markdown V1:
	// 1. Исправляем кейс с вложенными звёздочками, например: *Важное слово * другой текст* -> *Важное слово* другой текст*
	starRegex := regexp.MustCompile(`\*([^*]+) \*`)
	text = starRegex.ReplaceAllString(text, "*$1* ")

	// 2. Исправляем кейс с вложенными подчёркиваниями, например: _Важное слово _ другой текст_ -> _Важное слово_ другой текст_
	underscoreRegex := regexp.MustCompile(`_([^_]+) _`)
	text = underscoreRegex.ReplaceAllString(text, "_$1_ ")

	// 3. Заменяем MarkdownV2-специфические элементы на обычный текст
	text = strings.ReplaceAll(text, "~~", "") // зачеркивание не поддерживается
	text = strings.ReplaceAll(text, "||", "") // спойлеры не поддерживаются

	return text
}

// formatMessagesWithRoleMarkers форматирует сообщения с четким выделением ролей
// "ТЫ (УЧАСТНИК)" vs "СОБЕСЕДНИК" для лучшего понимания контекста LLM
func formatMessagesWithRoleMarkers(chatID int64, messages []*tgbotapi.Message, store storage.ChatHistoryStorage, cfg *config.Config, timeZone string, seenMessageIDs map[int]bool) string {
	var builder strings.Builder
	profiles := make(map[int64]*storage.UserProfile) // Кеш профилей для этого вызова
	loc, _ := time.LoadLocation(timeZone)

	// Получаем ID бота для определения роли
	var botUserID int64
	if globalBotInstance != nil && globalBotInstance.api != nil {
		botUserID = globalBotInstance.api.Self.ID
	}

	for _, msg := range messages {
		// Пропускаем дубликаты
		if seenMessageIDs[msg.MessageID] {
			continue
		}

		var authorAlias string
		var roleMarker string
		var profileInfo string

		if msg.From != nil {
			userID := msg.From.ID

			// Определяем роль с учетом расшифровки голоса
			if userID == botUserID {
				// Проверяем, является ли это расшифровкой голоса
				if mongoStorage, ok := store.(*storage.PostgresStorage); ok {
					// Получаем сообщение из MongoDB для проверки флага расшифровки
					if dbMessage, err := mongoStorage.GetMessageByID(chatID, msg.MessageID); err == nil && dbMessage != nil {
						// Получаем MongoMessage для проверки флага
						if mongoMessage, convertErr := mongoStorage.GetMongoMessageByID(chatID, msg.MessageID); convertErr == nil && mongoMessage != nil {
							if mongoMessage.IsVoiceTranscription && mongoMessage.OriginalVoiceUserID != 0 {
								// Это расшифровка голоса - обрабатываем как сообщение оригинального пользователя
								userID = mongoMessage.OriginalVoiceUserID
								roleMarker = "СОБЕСЕДНИК"

								// Получаем профиль оригинального пользователя для имени
								profile, found := profiles[userID]
								if !found {
									loadedProfile, err := store.GetUserProfile(chatID, userID)
									if err == nil && loadedProfile != nil {
										profiles[userID] = loadedProfile
										profile = loadedProfile
									}
								}

								// Определяем имя оригинального пользователя
								if profile != nil && profile.Alias != "" {
									authorAlias = profile.Alias
								} else {
									authorAlias = fmt.Sprintf("User_%d", userID)
								}

								// Добавляем маркер расшифровки голоса
								authorAlias = fmt.Sprintf("%s (расшифровка голоса)", authorAlias)
							} else {
								// Обычное сообщение от бота
								roleMarker = "ТЫ (УЧАСТНИК)"
								authorAlias = "Участник"
							}
						} else {
							// Если не удалось получить MongoMessage, считаем обычным сообщением бота
							roleMarker = "ТЫ (УЧАСТНИК)"
							authorAlias = "Участник"
						}
					} else {
						// Если не удалось получить сообщение из БД, считаем обычным сообщением бота
						roleMarker = "ТЫ (УЧАСТНИК)"
						authorAlias = "Участник"
					}
				} else {
					// Для не-MongoDB хранилищ считаем обычным сообщением бота
					roleMarker = "ТЫ (УЧАСТНИК)"
					authorAlias = "Участник"
				}
			} else {
				roleMarker = "СОБЕСЕДНИК"

				// Получаем профиль для обычного пользователя
				profile, found := profiles[userID]
				if !found {
					loadedProfile, err := store.GetUserProfile(chatID, userID)
					if err == nil && loadedProfile != nil {
						profiles[userID] = loadedProfile
						profile = loadedProfile
					}
				}

				// Определяем имя собеседника
				if profile != nil && profile.Alias != "" {
					authorAlias = profile.Alias
				} else if msg.From.FirstName != "" {
					authorAlias = msg.From.FirstName
				} else if msg.From.UserName != "" {
					authorAlias = msg.From.UserName
				} else {
					authorAlias = fmt.Sprintf("User_%d", userID)
				}

				// Добавляем информацию о профиле собеседника
				if profile != nil {
					profileInfoParts := []string{}
					if profile.Bio != "" {
						profileInfoParts = append(profileInfoParts, fmt.Sprintf("Bio: %s", utils.TruncateString(profile.Bio, 50)))
					}
					if profile.AutoBio != "" {
						profileInfoParts = append(profileInfoParts, fmt.Sprintf("AutoBio: %s", utils.TruncateString(profile.AutoBio, 50)))
					}
					if len(profileInfoParts) > 0 {
						profileInfo = fmt.Sprintf(" (%s)", strings.Join(profileInfoParts, "; "))
					}
				}
			}
		} else if msg.SenderChat != nil {
			roleMarker = "СОБЕСЕДНИК"
			authorAlias = msg.SenderChat.Title
			if authorAlias == "" {
				authorAlias = fmt.Sprintf("Chat_%d", msg.SenderChat.ID)
			}
		} else {
			roleMarker = "СОБЕСЕДНИК"
			authorAlias = "Неизвестный"
		}

		msgTime := time.Unix(int64(msg.Date), 0).In(loc)
		formattedTime := msgTime.Format("15:04:05")

		// Используем текст сообщения или подпись
		msgText := msg.Text
		if msgText == "" {
			msgText = msg.Caption
		}

		// Добавляем информацию о голосовом сообщении
		voiceIndicator := ""
		if msg.Voice != nil {
			voiceIndicator = "🗣️ "
		}

		// Добавляем информацию об ответе
		replyIndicator := ""
		if msg.ReplyToMessage != nil {
			replyIndicator = fmt.Sprintf(" (в ответ на #%d)", msg.ReplyToMessage.MessageID)
		}

		// Формируем строку с четким выделением роли
		builder.WriteString(fmt.Sprintf("%s (%s%s)%s%s:%s %s\n",
			formattedTime,
			roleMarker,
			profileInfo,    // Инфо о Bio только для собеседников
			replyIndicator, // Инфо об ответе
			voiceIndicator, // Индикатор голоса
			authorAlias,    // Имя
			msgText,
		))

		// Отмечаем ID как увиденный
		seenMessageIDs[msg.MessageID] = true
	}

	return builder.String()
}

// formatUnifiedContextWithMetadata - универсальная функция форматирования контекста с полными метаданными
// Используется для всех типов ответов (direct, general, free will, voice) для единообразного понимания контекста
func formatUnifiedContextWithMetadata(chatID int64, messages []*tgbotapi.Message, relevantMessages []*tgbotapi.Message, store storage.ChatHistoryStorage, cfg *config.Config, timeZone string, contextType string) string {
	log.Printf("🚀 [CONTEXT] Начинаем формирование контекста для чата %d. Сообщений: %d, релевантных: %d, тип: %s",
		chatID, len(messages), len(relevantMessages), contextType)

	// Проверяем какой режим форматирования активен
	if cfg != nil && cfg.UseStructuredMessageFormat {
		log.Printf("🏗️ [CONTEXT] Используется СТРУКТУРИРОВАННОЕ форматирование сообщений с тегами [MSG_START]/[MSG_END]")
	} else {
		log.Printf("📝 [CONTEXT] Используется СТАНДАРТНОЕ форматирование сообщений")
	}

	// Настраиваем локальное время
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		log.Printf("[ERROR] Ошибка загрузки часового пояса: %v, используем UTC", err)
		loc = time.UTC
	}

	var sb strings.Builder

	// ИСПРАВЛЕНИЕ: Создаем унифицированный кеш профилей для всех сообщений
	profiles := make(map[int64]*storage.UserProfile)

	// Предварительно загружаем профили для всех пользователей, участвующих в контексте
	userIDs := make(map[int64]bool)
	for _, msg := range messages {
		if msg.From != nil {
			userIDs[msg.From.ID] = true
		}
	}
	for _, msg := range relevantMessages {
		if msg.From != nil {
			userIDs[msg.From.ID] = true
		}
	}

	// Загружаем все профили сразу для обеспечения консистентности
	for userID := range userIDs {
		profile, err := store.GetUserProfile(chatID, userID)
		if err != nil {
			log.Printf("[WARN] Chat %d: Ошибка загрузки профиля для userID %d: %v", chatID, userID, err)
		} else if profile != nil {
			profiles[userID] = profile
			log.Printf("[DEBUG] Chat %d: Загружен профиль для userID %d (алиас: %s)", chatID, userID, profile.Alias)
		}
	}

	log.Printf("[INFO] Chat %d: Предварительно загружено %d профилей для обеспечения консистентности", chatID, len(profiles))

	// НОВОЕ: Обновляем кеш дисамбигуации пользователей если доступен валидатор
	if globalBotInstance != nil && globalBotInstance.userValidator != nil {
		if err := globalBotInstance.userValidator.UpdateChatProfiles(chatID); err != nil {
			log.Printf("[WARN] Ошибка обновления кеша дисамбигуации для чата %d: %v", chatID, err)
		}

		// НОВОЕ: Проверяем конфликты алиасов и добавляем предупреждения
		conflicts := globalBotInstance.userValidator.CheckAliasConflicts(chatID)
		if len(conflicts) > 0 {
			conflictWarning := globalBotInstance.userValidator.LogConflictWarning(chatID)
			if conflictWarning != "" {
				sb.WriteString("=== ВНИМАНИЕ: КОНФЛИКТЫ ПОЛЬЗОВАТЕЛЕЙ ===\n")
				sb.WriteString(conflictWarning)
				sb.WriteString("=== КОНЕЦ ПРЕДУПРЕЖДЕНИЯ ===\n\n")
			}
		}
	}

	// 1. Добавляем текущее время и тип контекста
	now := time.Now().In(loc)
	sb.WriteString(fmt.Sprintf("Текущее время: %s\n", now.Format("15:04 02.01.2006")))
	sb.WriteString(fmt.Sprintf("Тип взаимодействия: %s\n\n", contextType))

	// 1.1 Добавляем специальный временной контекст для саммари
	if contextType == "general" || contextType == "summary" {
		// Для саммари добавляем подробную временную ориентацию
		sb.WriteString("=== ВРЕМЕННОЙ КОНТЕКСТ ===\n")
		sb.WriteString(fmt.Sprintf("Текущий день недели: %s\n", now.Format("Monday")))
		sb.WriteString(fmt.Sprintf("Текущая дата: %s\n", now.Format("02.01.2006")))
		sb.WriteString(fmt.Sprintf("Период анализа: последние 24 часа (с %s по %s)\n",
			now.Add(-24*time.Hour).Format("02.01.2006 15:04"),
			now.Format("02.01.2006 15:04")))
		sb.WriteString("\nВРЕМЕННЫЕ ПЕРИОДЫ:\n")
		sb.WriteString("• Утро: 06:00-12:00\n")
		sb.WriteString("• День: 12:00-18:00\n")
		sb.WriteString("• Вечер: 18:00-00:00\n")
		sb.WriteString("• Ночь: 00:00-06:00 (следующий день!)\n")

		if contextType == "summary" {
			sb.WriteString("\n🚨 КРИТИЧЕСКИ ВАЖНО ДЛЯ САММАРИ:\n")
			sb.WriteString("ХРОНОЛОГИЧЕСКАЯ СОРТИРОВКА ВРЕМЕН СУТОК:\n")
			sb.WriteString("- Используй временные метки в формате 15:04(DayOfWeek,DD.MM) для определения правильного порядка\n")
			sb.WriteString("- Сначала сравнивай ДАТУ (DD.MM), затем время суток\n")
			sb.WriteString("- В одной дате: 🌙 НОЧЬ → 🌅 УТРО → ☀️ ДЕНЬ → 🌆 ВЕЧЕР\n")
			sb.WriteString("- Между разными датами: соблюдай хронологию дат\n")
			sb.WriteString("- ОБЯЗАТЕЛЬНО используй эмодзи: 🌙 🌅 ☀️ 🌆 для оформления времен суток\n")
			sb.WriteString("- Пример правильного порядка:\n")
			sb.WriteString("  🌙 НОЧЬ (01.12): события 00:00-06:00\n")
			sb.WriteString("  🌅 УТРО (01.12): события 06:00-12:00\n")
			sb.WriteString("  ☀️ ДЕНЬ (01.12): события 12:00-18:00\n")
			sb.WriteString("  🌆 ВЕЧЕР (01.12): события 18:00-00:00\n")
			sb.WriteString("  🌙 НОЧЬ (02.12): события 00:00-06:00 СЛЕДУЮЩЕГО дня\n")
		} else {
			sb.WriteString("\nВНИМАНИЕ: Строго соблюдайте хронологию! Событие в 23:30 = ВЕЧЕР, событие в 02:00 = НОЧЬ.\n")
		}
		sb.WriteString("=== КОНЕЦ ВРЕМЕННОГО КОНТЕКСТА ===\n\n")
	}

	// 2. Добавляем информацию о личности бота (PersonalityMemory)
	memory, err := store.GetPersonalityMemory(chatID)
	if err == nil && memory != nil {
		sb.WriteString("=== КОНТЕКСТ ЛИЧНОСТИ ===\n")

		// Самоидентификация
		if len(memory.SelfPerception) > 0 {
			sb.WriteString("Твоё самоощущение:\n")
			for _, perception := range memory.SelfPerception {
				sb.WriteString(fmt.Sprintf("- %s\n", perception))
			}
		}

		// Важные имена
		if len(memory.NameMentions) > 0 {
			sb.WriteString("Важные имена в чате: ")
			var names []string
			for name := range memory.NameMentions {
				names = append(names, name)
			}
			sb.WriteString(strings.Join(names, ", "))
			sb.WriteString("\n")
		}

		// Недавние темы
		if len(memory.RecentTopics) > 0 {
			sb.WriteString("Недавние темы: ")
			sb.WriteString(strings.Join(memory.RecentTopics, ", "))
			sb.WriteString("\n")
		}

		// Текущие темы обсуждения
		if len(memory.DiscussionContext) > 0 {
			sb.WriteString("Обсуждаем сейчас: ")
			var topics []string
			for topic := range memory.DiscussionContext {
				topics = append(topics, topic)
			}
			sb.WriteString(strings.Join(topics, ", "))
			sb.WriteString("\n")
		}

		sb.WriteString("=== КОНЕЦ ИНФОРМАЦИИ О ЛИЧНОСТИ ===\n\n")
	}

	// 3. Добавляем релевантные сообщения из прошлого, если есть
	if len(relevantMessages) > 0 {
		sb.WriteString("=== РЕЛЕВАНТНАЯ ПРЕДЫСТОРИЯ ===\n")
		for _, msg := range relevantMessages {
			messageStr := formatSingleMessageWithMetadata(chatID, msg, profiles, store, loc, contextType)
			sb.WriteString(messageStr)
		}
		sb.WriteString("\n")
	}

	// 4. Добавляем основную историю сообщений с полными метаданными
	if contextType == "summary" {
		// Для саммари группируем сообщения по времени суток с учетом дат в правильном хронологическом порядке
		sb.WriteString("=== ИСТОРИЯ СООБЩЕНИЙ СГРУППИРОВАННАЯ ПО ВРЕМЕНИ СУТОК ===\n")

		// Структура для хранения сообщений с дополнительной информацией
		type MessageWithTimeInfo struct {
			msg            *tgbotapi.Message
			msgTime        time.Time
			dateKey        string // YYYY-MM-DD
			timeGroup      string // night, morning, day, evening
			timeGroupOrder int    // 0=night, 1=morning, 2=day, 3=evening
		}

		// Группируем сообщения по времени суток с учетом дат
		var messagesWithTime []MessageWithTimeInfo

		groupTitles := map[string]string{
			"night":   "🌙 НОЧЬ (00:00-06:00)",
			"morning": "🌅 УТРО (06:00-12:00)",
			"day":     "☀️ ДЕНЬ (12:00-18:00)",
			"evening": "🌆 ВЕЧЕР (18:00-00:00)",
		}

		groupOrder := map[string]int{
			"night":   0,
			"morning": 1,
			"day":     2,
			"evening": 3,
		}

		// Обрабатываем каждое сообщение
		for _, msg := range messages {
			msgTime := time.Unix(int64(msg.Date), 0).In(loc)
			dateKey := msgTime.Format("2006-01-02")
			hour := msgTime.Hour()

			var timeGroup string
			if hour >= 0 && hour < 6 { // 00:00-05:59
				timeGroup = "night"
			} else if hour >= 6 && hour < 12 { // 06:00-11:59
				timeGroup = "morning"
			} else if hour >= 12 && hour < 18 { // 12:00-17:59
				timeGroup = "day"
			} else { // 18:00-23:59
				timeGroup = "evening"
			}

			messagesWithTime = append(messagesWithTime, MessageWithTimeInfo{
				msg:            msg,
				msgTime:        msgTime,
				dateKey:        dateKey,
				timeGroup:      timeGroup,
				timeGroupOrder: groupOrder[timeGroup],
			})
		}

		// Сортируем сообщения по времени (хронологически)
		sort.Slice(messagesWithTime, func(i, j int) bool {
			return messagesWithTime[i].msgTime.Before(messagesWithTime[j].msgTime)
		})

		// Группируем по дате + времени суток в хронологическом порядке
		type DateTimeGroup struct {
			dateKey   string
			timeGroup string
			title     string
			messages  []MessageWithTimeInfo
		}

		var chronologicalGroups []DateTimeGroup
		currentGroup := DateTimeGroup{}

		for _, msgInfo := range messagesWithTime {
			// Если это новая группа (дата + время суток)
			if currentGroup.dateKey != msgInfo.dateKey || currentGroup.timeGroup != msgInfo.timeGroup {
				// Сохраняем предыдущую группу, если она не пустая
				if len(currentGroup.messages) > 0 {
					chronologicalGroups = append(chronologicalGroups, currentGroup)
				}

				// Создаем новую группу
				dateFormatted := msgInfo.msgTime.Format("02.01")
				currentGroup = DateTimeGroup{
					dateKey:   msgInfo.dateKey,
					timeGroup: msgInfo.timeGroup,
					title:     fmt.Sprintf("%s (%s)", groupTitles[msgInfo.timeGroup], dateFormatted),
					messages:  []MessageWithTimeInfo{msgInfo},
				}
			} else {
				// Добавляем сообщение в текущую группу
				currentGroup.messages = append(currentGroup.messages, msgInfo)
			}
		}

		// Добавляем последнюю группу
		if len(currentGroup.messages) > 0 {
			chronologicalGroups = append(chronologicalGroups, currentGroup)
		}

		// Выводим группы в хронологическом порядке
		for _, group := range chronologicalGroups {
			sb.WriteString(fmt.Sprintf("\n%s:\n", group.title))
			for _, msgInfo := range group.messages {
				messageStr := formatSingleMessageWithMetadata(chatID, msgInfo.msg, profiles, store, loc, contextType)
				sb.WriteString(messageStr)
			}
		}
	} else {
		// Для остальных контекстов - обычная хронология
		sb.WriteString("=== ИСТОРИЯ СООБЩЕНИЙ С МЕТАДАННЫМИ ===\n")
		for _, msg := range messages {
			messageStr := formatSingleMessageWithMetadata(chatID, msg, profiles, store, loc, contextType)
			sb.WriteString(messageStr)
		}
	}

	// 5. Добавляем инструкции в зависимости от типа контекста
	switch contextType {
	case "decision_making":
		// Для Free Will КРИТИЧНО видеть MessageID - оставляем техническую инструкцию
		sb.WriteString("\nДля принятия решений используй MessageID из колонки [MSG:ID]\n")
	case "direct_reply":
		sb.WriteString("\nЭто прямое обращение к тебе - отвечай персонально и учитывай весь контекст\n")
		sb.WriteString("КРИТИЧНО: Убедись, что обращаешься к правильному пользователю! Проверь алиас автора сообщения.\n")
	case "general":
		sb.WriteString("\nАнализируй общую атмосферу и контекст для подходящего ответа\n")
	case "voice":
		sb.WriteString("\nФормируй естественную речь для голосового сообщения, учитывая контекст\n")
	case "summary":
		sb.WriteString("\nСоздай структурированное саммари с хронологической сортировкой по времени суток на основе временных меток!\n")
	}

	result := sb.String()
	log.Printf("✅ [CONTEXT] Контекст сформирован. Итоговая длина: %d символов", len(result))
	log.Printf("📊 [CONTEXT] Статистика контекста:")
	log.Printf("   - Сообщений обработано: %d", len(messages))
	log.Printf("   - Релевантных сообщений: %d", len(relevantMessages))
	log.Printf("   - Тип контекста: %s", contextType)
	log.Printf("   - Часовой пояс: %s", timeZone)
	log.Printf("   - Профилей в кеше: %d", len(profiles))

	return result
}

// formatSingleMessageWithMetadata форматирует одно сообщение с полными метаданными
func formatSingleMessageWithMetadata(chatID int64, msg *tgbotapi.Message, profilesCache map[int64]*storage.UserProfile, store storage.ChatHistoryStorage, loc *time.Location, contextType string) string {
	if msg == nil {
		return ""
	}

	// НОВОЕ: Проверяем настройку структурированного форматирования
	if globalBotInstance != nil && globalBotInstance.config != nil && globalBotInstance.config.UseStructuredMessageFormat {
		log.Printf("🔄 [STRUCTURED_FORMAT] Используется новый структурированный формат для сообщения ID:%d от пользователя ID:%d в чате %d", msg.MessageID, msg.From.ID, chatID)
		result := formatStructuredMessage(chatID, msg, profilesCache, store, loc, contextType)
		log.Printf("✅ [STRUCTURED_FORMAT] Результат форматирования (длина: %d символов): %.200s...", len(result), result)
		return result
	}

	log.Printf("📝 [LEGACY_FORMAT] Используется старый формат для сообщения ID:%d от пользователя ID:%d в чате %d", msg.MessageID, msg.From.ID, chatID)

	// СТАРЫЙ ФОРМАТ: Оставляем существующую логику без изменений
	// Получаем автора сообщения
	var authorAlias string
	var authorBio string
	var authorAutoBio string
	var profileInfo string

	if msg.From != nil {
		userID := msg.From.ID

		// ИСПРАВЛЕНИЕ: Используем уже загруженный профиль из кеша - больше не дублируем загрузку
		profile, found := profilesCache[userID]
		if !found {
			// Этого не должно произойти, если предварительная загрузка работает правильно
			log.Printf("[WARN] Chat %d: Профиль для userID %d не найден в предварительно загруженном кеше", chatID, userID)
			// Загружаем профиль как fallback
			loadedProfile, err := store.GetUserProfile(chatID, userID)
			if err != nil {
				log.Printf("[ERROR] Chat %d: Ошибка загрузки профиля для userID %d: %v", chatID, userID, err)
			} else if loadedProfile != nil {
				profilesCache[userID] = loadedProfile
				profile = loadedProfile
			}
		}

		// УЛУЧШЕНИЕ: Используем систему дисамбигуации для определения алиаса с улучшенной обработкой ошибок
		if globalBotInstance != nil && globalBotInstance.userValidator != nil {
			authorAlias = globalBotInstance.userValidator.FormatUserWithDisambiguation(chatID, userID, contextType, msg)
			log.Printf("[DEBUG] Chat %d: Использован алиас из дисамбигуации: %s для userID %d", chatID, authorAlias, userID)
		} else {
			// УЛУЧШЕННЫЙ Fallback - более надежная логика при отсутствии валидатора
			log.Printf("[WARN] Chat %d: Валидатор недоступен для userID %d, используем улучшенный fallback", chatID, userID)

			// Приоритет: Alias > FirstName > UserName > ID
			if profile != nil && profile.Alias != "" {
				authorAlias = profile.Alias
				// Для критичных контекстов добавляем ID для дисамбигуации
				if contextType == "direct_reply" {
					authorAlias = fmt.Sprintf("%s@U%d", authorAlias, userID)
				}
			} else if msg.From.FirstName != "" {
				authorAlias = msg.From.FirstName
				// Для критичных контекстов добавляем ID для дисамбигуации
				if contextType == "direct_reply" {
					authorAlias = fmt.Sprintf("%s@U%d", authorAlias, userID)
				}
			} else if msg.From.UserName != "" {
				authorAlias = msg.From.UserName
				if contextType == "direct_reply" {
					authorAlias = fmt.Sprintf("%s@U%d", authorAlias, userID)
				}
			} else {
				authorAlias = fmt.Sprintf("User_%d@U%d", userID, userID)
			}

			log.Printf("[DEBUG] Chat %d: Использован fallback алиас: %s для userID %d", chatID, authorAlias, userID)
		}

		// Получаем Bio, если есть (сокращенно для контекста)
		if profile != nil && profile.Bio != "" {
			authorBio = utils.TruncateString(profile.Bio, 50)
		}

		// Получаем AutoBio, если есть (сокращенно для контекста)
		if profile != nil && profile.AutoBio != "" {
			authorAutoBio = utils.TruncateString(profile.AutoBio, 50)
		}

		// Формируем краткую строку с профилем (только для контекста, не для вывода)
		profileInfoParts := []string{}
		if authorBio != "" {
			profileInfoParts = append(profileInfoParts, fmt.Sprintf("Bio:%s", authorBio))
		}
		if authorAutoBio != "" {
			profileInfoParts = append(profileInfoParts, fmt.Sprintf("AutoBio:%s", authorAutoBio))
		}
		if len(profileInfoParts) > 0 {
			profileInfo = fmt.Sprintf(" (%s)", strings.Join(profileInfoParts, ";"))
		}
	} else if msg.SenderChat != nil {
		authorAlias = msg.SenderChat.Title
		if authorAlias == "" {
			authorAlias = fmt.Sprintf("Chat_%d", msg.SenderChat.ID)
		}
	} else {
		authorAlias = "Неизвестный"
		log.Printf("[WARN] Chat %d: Сообщение ID %d не имеет автора", chatID, msg.MessageID)
	}

	// Форматируем время (более человечно)
	msgTime := time.Unix(int64(msg.Date), 0).In(loc)
	timeStr := msgTime.Format("15:04")
	dateStr := msgTime.Format("02.01")

	// Добавляем день недели для лучшей хронологической ориентации
	dayOfWeek := msgTime.Format("Mon") // Пн, Вт, Ср, Чт, Пт, Сб, Вс

	// Формируем полную временную метку с днем недели
	fullTimeStr := fmt.Sprintf("%s(%s,%s)", timeStr, dayOfWeek, dateStr)

	// Получаем текст сообщения
	msgText := msg.Text
	if msgText == "" {
		msgText = msg.Caption
	}

	// Добавляем метаданные о типе сообщения
	var typeIndicators []string

	// Голосовое сообщение
	if msg.Voice != nil {
		typeIndicators = append(typeIndicators, "🗣️ГОЛОС")
	}

	// Фото/видео/документы
	if len(msg.Photo) > 0 {
		typeIndicators = append(typeIndicators, "📷ФОТО")
	}
	if msg.Video != nil {
		typeIndicators = append(typeIndicators, "🎥ВИДЕО")
	}
	if msg.Document != nil {
		typeIndicators = append(typeIndicators, "📎ДОК")
	}
	if msg.Audio != nil {
		typeIndicators = append(typeIndicators, "🎵АУДИО")
	}
	if msg.Sticker != nil {
		typeIndicators = append(typeIndicators, "🎭СТИКЕР")
	}

	// Пересланное сообщение
	if msg.ForwardFrom != nil || msg.ForwardFromChat != nil {
		forwardInfo := "ПЕРЕСЛ"
		if msg.ForwardFrom != nil {
			forwardInfo += fmt.Sprintf("(от:%s)", getForwardFromName(msg.ForwardFrom))
		} else if msg.ForwardFromChat != nil {
			forwardInfo += fmt.Sprintf("(из:%s)", msg.ForwardFromChat.Title)
		}
		typeIndicators = append(typeIndicators, forwardInfo)
	}

	// УЛУЧШЕННАЯ обработка ответов на сообщения с дисамбигуацией
	replyInfo := ""
	if msg.ReplyToMessage != nil {
		if globalBotInstance != nil && globalBotInstance.userValidator != nil {
			replyInfo = " " + globalBotInstance.userValidator.FormatReplyReference(chatID, msg.ReplyToMessage)
			log.Printf("[DEBUG] Chat %d: Использована дисамбигуация для reply: %s", chatID, replyInfo)
		} else {
			// УЛУЧШЕННЫЙ Fallback для reply references
			replyToAuthor := "Неизвестный"
			replyToUserID := int64(0)

			if msg.ReplyToMessage.From != nil {
				replyToUserID = msg.ReplyToMessage.From.ID

				// Проверяем кеш профилей для автора исходного сообщения
				if replyProfile, found := profilesCache[replyToUserID]; found && replyProfile != nil && replyProfile.Alias != "" {
					replyToAuthor = replyProfile.Alias
				} else if msg.ReplyToMessage.From.FirstName != "" {
					replyToAuthor = msg.ReplyToMessage.From.FirstName
				} else if msg.ReplyToMessage.From.UserName != "" {
					replyToAuthor = msg.ReplyToMessage.From.UserName
				} else {
					replyToAuthor = fmt.Sprintf("User_%d", replyToUserID)
				}

				// Для критичных контекстов добавляем ID для дисамбигуации
				if contextType == "direct_reply" {
					replyToAuthor = fmt.Sprintf("%s@U%d", replyToAuthor, replyToUserID)
				}
			}

			replyInfo = fmt.Sprintf(" ↪️ОТВЕТ_НА[MSG:%d](%s)", msg.ReplyToMessage.MessageID, replyToAuthor)
			log.Printf("[DEBUG] Chat %d: Использован fallback для reply: %s", chatID, replyInfo)
		}
	}

	// Собираем все индикаторы
	typeIndicatorStr := ""
	if len(typeIndicators) > 0 {
		typeIndicatorStr = "[" + strings.Join(typeIndicators, ",") + "]"
	}

	// Формируем итоговую строку в зависимости от типа контекста
	var result string
	if contextType == "decision_making" {
		// Для принятия решений Free Will - акцент на MessageID (единственное исключение)
		result = fmt.Sprintf("[MSG:%d] %s %s%s%s: %s%s\n",
			msg.MessageID,
			fullTimeStr,
			authorAlias,
			profileInfo,
			replyInfo,
			typeIndicatorStr,
			msgText,
		)
	} else {
		// Для остальных контекстов - условно показываем ID (только для Free Will)
		result = fmt.Sprintf("%s %s%s%s%s: %s%s\n",
			fullTimeStr,
			authorAlias,
			profileInfo,
			replyInfo,
			typeIndicatorStr,
			msgText,
			getMessageIDForContext(msg.MessageID, contextType),
		)
	}

	return result
}

// getForwardFromName получает имя пользователя, от которого переслано сообщение
func getForwardFromName(user *tgbotapi.User) string {
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.UserName != "" {
		return user.UserName
	}
	return fmt.Sprintf("User_%d", user.ID)
}

// getMessageIDForContext возвращает ID сообщения для контекста (скрытый от основного текста)
func getMessageIDForContext(messageID int, contextType string) string {
	// Показываем ID только для Free Will decision_making - там это критично
	if contextType == "decision_making" {
		return fmt.Sprintf(" {ID:%d}", messageID)
	}
	// Для остальных контекстов НЕ показываем ID - скрываем ботовость
	return ""
}

// formatStructuredMessage форматирует одно сообщение в структурированном виде с тегами [MSG] и метаданными
// Этот формат помогает LLM лучше понимать структуру сообщений и их метаданные
func formatStructuredMessage(chatID int64, msg *tgbotapi.Message, profilesCache map[int64]*storage.UserProfile, store storage.ChatHistoryStorage, loc *time.Location, contextType string) string {
	log.Printf("🏗️ [STRUCT_MSG] Начинаем структурированное форматирование сообщения ID:%d", msg.MessageID)

	// Получаем автора с использованием системы дисамбигуации
	var userIDTag string
	if msg.From != nil {
		userID := msg.From.ID
		if globalBotInstance != nil && globalBotInstance.userValidator != nil {
			userIDTag = globalBotInstance.userValidator.FormatUserWithDisambiguation(chatID, userID, contextType, msg)
			log.Printf("👤 [STRUCT_MSG] Использована дисамбигуация для автора: %s", userIDTag)
		} else {
			// Fallback без дисамбигуации - используем алиас из профиля
			displayName := fmt.Sprintf("User_%d", userID)
			if profile, exists := profilesCache[userID]; exists && profile != nil {
				if profile.Alias != "" {
					displayName = profile.Alias
				} else if profile.RealName != "" {
					displayName = profile.RealName
				} else if profile.Username != "" {
					displayName = profile.Username
				}
			}
			userIDTag = fmt.Sprintf("[MSG:%d][U%d:%s]", msg.MessageID, userID, displayName)
			log.Printf("👤 [STRUCT_MSG] Использован fallback для автора: %s", userIDTag)
		}
	} else {
		log.Printf("⚠️ [STRUCT_MSG] Сообщение без автора, используется System")
	}

	// Собираем метаданные сообщения
	var metadata []string

	// Время и дата
	msgTime := time.Unix(int64(msg.Date), 0).In(loc)
	timeStr := msgTime.Format("15:04")
	dateStr := msgTime.Format("02.01 (Mon)")
	metadata = append(metadata, fmt.Sprintf("Время: %s", timeStr))
	metadata = append(metadata, fmt.Sprintf("Дата: %s", dateStr))
	log.Printf("⏰ [STRUCT_MSG] Добавлено время: %s, дата: %s", timeStr, dateStr)

	// Биография пользователя
	if msg.From != nil {
		if profile, exists := profilesCache[msg.From.ID]; exists && profile != nil && profile.Bio != "" {
			metadata = append(metadata, fmt.Sprintf("Bio: %s", profile.Bio))
			log.Printf("📝 [STRUCT_MSG] Добавлена биография: %.50s...", profile.Bio)
		}
	}

	// Тип сообщения и источник
	var messageTypes []string
	if msg.From != nil && msg.From.IsBot {
		messageTypes = append(messageTypes, "бот")
		log.Printf("🤖 [STRUCT_MSG] Сообщение от бота")
	} else {
		messageTypes = append(messageTypes, "пользователь")
		log.Printf("👨‍💻 [STRUCT_MSG] Сообщение от пользователя")
	}

	// Определяем тип контента
	if msg.Voice != nil {
		messageTypes = append(messageTypes, "голосовое")
		log.Printf("🗣️ [STRUCT_MSG] Голосовое сообщение")
	} else if len(msg.Photo) > 0 {
		messageTypes = append(messageTypes, "изображение")
		log.Printf("🖼️ [STRUCT_MSG] Сообщение с изображением")
	} else if msg.Document != nil {
		messageTypes = append(messageTypes, "документ")
		log.Printf("📄 [STRUCT_MSG] Сообщение с документом")
	} else if msg.Audio != nil {
		messageTypes = append(messageTypes, "аудио")
		log.Printf("🎵 [STRUCT_MSG] Аудио сообщение")
	} else if msg.Video != nil {
		messageTypes = append(messageTypes, "видео")
		log.Printf("🎥 [STRUCT_MSG] Видео сообщение")
	} else if msg.Sticker != nil {
		messageTypes = append(messageTypes, "стикер")
		log.Printf("😄 [STRUCT_MSG] Стикер")
	} else {
		messageTypes = append(messageTypes, "текстовое")
		log.Printf("💬 [STRUCT_MSG] Текстовое сообщение")
	}
	metadata = append(metadata, fmt.Sprintf("Тип: %s", strings.Join(messageTypes, ", ")))

	// Обработка пересланных сообщений
	if msg.ForwardFrom != nil || msg.ForwardFromChat != nil {
		var forwardInfo string
		if msg.ForwardFrom != nil {
			forwardName := getForwardFromName(msg.ForwardFrom)
			forwardInfo = fmt.Sprintf("Переслано от: %s", forwardName)
			log.Printf("📤 [STRUCT_MSG] Переслано от пользователя: %s", forwardName)
		} else if msg.ForwardFromChat != nil {
			forwardInfo = fmt.Sprintf("Переслано из: %s", msg.ForwardFromChat.Title)
			log.Printf("📤 [STRUCT_MSG] Переслано из чата: %s", msg.ForwardFromChat.Title)
		}
		metadata = append(metadata, forwardInfo)
	}

	// Информация об ответе на сообщение
	if msg.ReplyToMessage != nil {
		replyInfo := fmt.Sprintf("Ответ на сообщение: #%d", msg.ReplyToMessage.MessageID)
		metadata = append(metadata, replyInfo)
		log.Printf("↩️ [STRUCT_MSG] Ответ на сообщение ID: %d", msg.ReplyToMessage.MessageID)

		// Добавляем автора исходного сообщения с дисамбигуацией
		if msg.ReplyToMessage.From != nil {
			var originalAuthorDisplay string
			originalUserID := msg.ReplyToMessage.From.ID

			if globalBotInstance != nil && globalBotInstance.userValidator != nil {
				// Используем систему дисамбигуации для автора исходного сообщения
				originalAuthorDisplay = globalBotInstance.userValidator.FormatUserWithDisambiguation(chatID, originalUserID, contextType, msg.ReplyToMessage)
				log.Printf("👤 [STRUCT_MSG] Автор исходного сообщения (дисамбигуация): %s", originalAuthorDisplay)
			} else {
				// Fallback для автора исходного сообщения
				displayName := fmt.Sprintf("User_%d", originalUserID)
				if profile, exists := profilesCache[originalUserID]; exists && profile != nil {
					if profile.Alias != "" {
						displayName = profile.Alias
					} else if profile.RealName != "" {
						displayName = profile.RealName
					} else if profile.Username != "" {
						displayName = profile.Username
					}
				}
				originalAuthorDisplay = displayName
				log.Printf("👤 [STRUCT_MSG] Автор исходного сообщения (fallback): %s", originalAuthorDisplay)
			}
			metadata = append(metadata, fmt.Sprintf("Автор исходного сообщения: %s", originalAuthorDisplay))
		}
	}

	// Основной текст сообщения
	var messageText string
	if msg.Text != "" {
		messageText = msg.Text
	} else if msg.Caption != "" {
		messageText = msg.Caption
	} else {
		// Для специальных типов сообщений
		if msg.Voice != nil {
			messageText = "[голосовое сообщение]"
		} else if len(msg.Photo) > 0 {
			messageText = "[изображение]"
		} else if msg.Document != nil {
			messageText = fmt.Sprintf("[документ: %s]", msg.Document.FileName)
		} else if msg.Audio != nil {
			messageText = "[аудио файл]"
		} else if msg.Video != nil {
			messageText = "[видео]"
		} else if msg.Sticker != nil {
			messageText = "[стикер]"
		} else {
			messageText = "[специальное сообщение]"
		}
	}
	log.Printf("💭 [STRUCT_MSG] Текст сообщения (длина: %d): %.100s...", len(messageText), messageText)

	// Формируем структурированное сообщение
	var sb strings.Builder

	// Открывающий тег
	sb.WriteString("[MSG]\n")

	// Метаданные
	for _, meta := range metadata {
		sb.WriteString(meta + "\n")
	}

	// Основной текст
	sb.WriteString("\nТекст: " + messageText + "\n")

	// Закрывающий тег
	sb.WriteString("[/MSG]\n")

	result := sb.String()
	log.Printf("✅ [STRUCT_MSG] Структурированное форматирование завершено. Итоговая длина: %d символов", len(result))

	return result
}

// determineContextType определяет подходящий тип контекста на основе сценария использования
func determineContextType(scenario string, message *tgbotapi.Message, isDirectMention bool, isReplyToBot bool) string {
	switch scenario {
	case "free_will_decision":
		return "decision_making"
	case "voice_message":
		return "voice"
	case "summary", "daily_summary", "weekly_summary":
		return "summary"
	case "direct_response":
		// Для прямых ответов всегда используем critical контекст для предотвращения путаницы
		return "direct_reply"
	case "general_ai_response":
		// Для общих AI ответов используем общий контекст
		return "general"
	case "moderation":
		return "general"
	case "reply_chain":
		// Для цепочек ответов используем критический контекст
		if isDirectMention || isReplyToBot {
			return "direct_reply"
		}
		return "general"
	default:
		log.Printf("[WARN] Неизвестный сценарий контекста: %s, используем 'general'", scenario)
		return "general"
	}
}

// validateContextType проверяет и корректирует тип контекста если необходимо
func validateContextType(contextType string, message *tgbotapi.Message, isDirectMention bool, isReplyToBot bool) string {
	validTypes := map[string]bool{
		"decision_making": true,
		"direct_reply":    true,
		"general":         true,
		"voice":           true,
		"summary":         true,
	}

	if !validTypes[contextType] {
		log.Printf("[WARN] Недопустимый тип контекста: %s, используем 'general'", contextType)
		return "general"
	}

	// Дополнительная проверка: если это прямое упоминание или ответ на бота, но контекст не критичный - корректируем
	if (isDirectMention || isReplyToBot) && contextType == "general" {
		log.Printf("[INFO] Корректируем контекст с 'general' на 'direct_reply' для критичного взаимодействия")
		return "direct_reply"
	}

	return contextType
}

// validateReplyChain проверяет корректность идентификации пользователей в цепочке ответов
func validateReplyChain(chatID int64, replyChain []*tgbotapi.Message, store storage.ChatHistoryStorage) ([]*tgbotapi.Message, []string) {
	if len(replyChain) == 0 {
		return replyChain, nil
	}

	var warnings []string
	validatedChain := make([]*tgbotapi.Message, 0, len(replyChain))
	userMap := make(map[int64]bool) // Отслеживание пользователей в цепочке

	log.Printf("[REPLY_VALIDATION] Chat %d: Валидация цепочки ответов из %d сообщений", chatID, len(replyChain))

	for i, msg := range replyChain {
		if msg == nil {
			log.Printf("[REPLY_VALIDATION] Chat %d: Сообщение %d в цепочке является nil, пропускаем", chatID, i)
			warnings = append(warnings, fmt.Sprintf("Сообщение %d в цепочке повреждено", i))
			continue
		}

		// Проверяем наличие автора сообщения
		if msg.From == nil {
			log.Printf("[REPLY_VALIDATION] Chat %d: Сообщение %d не имеет автора, пропускаем", chatID, msg.MessageID)
			warnings = append(warnings, fmt.Sprintf("Сообщение %d не имеет автора", msg.MessageID))
			continue
		}

		userID := msg.From.ID
		userMap[userID] = true

		// Проверяем корректность связи с предыдущим сообщением
		if msg.ReplyToMessage != nil {
			expectedPrevID := msg.ReplyToMessage.MessageID
			prevFound := false

			for j := 0; j < len(validatedChain); j++ {
				if validatedChain[j].MessageID == expectedPrevID {
					prevFound = true
					break
				}
			}

			if !prevFound && i > 0 {
				log.Printf("[REPLY_VALIDATION] Chat %d: Нарушена связь в цепочке ответов на сообщении %d", chatID, msg.MessageID)
				warnings = append(warnings, fmt.Sprintf("Нарушена связь цепочки на сообщении %d", msg.MessageID))
			}
		}

		validatedChain = append(validatedChain, msg)
	}

	// Проверяем на подозрительные шаблоны
	if len(userMap) == 1 {
		log.Printf("[REPLY_VALIDATION] Chat %d: Вся цепочка ответов содержит сообщения от одного пользователя - возможна путаница", chatID)
		warnings = append(warnings, "Вся цепочка ответов от одного пользователя")
	}

	log.Printf("[REPLY_VALIDATION] Chat %d: Валидация завершена. Исходно: %d, валидно: %d, предупреждений: %d",
		chatID, len(replyChain), len(validatedChain), len(warnings))

	return validatedChain, warnings
}

// validateReplyReference проверяет корректность ссылки на сообщение в reply
func validateReplyReference(chatID int64, replyMsg *tgbotapi.Message, profilesCache map[int64]*storage.UserProfile, store storage.ChatHistoryStorage) (string, error) {
	if replyMsg == nil {
		return "", fmt.Errorf("reply message is nil")
	}

	if replyMsg.From == nil {
		return "", fmt.Errorf("reply message has no author")
	}

	userID := replyMsg.From.ID

	// Проверяем есть ли профиль пользователя в кеше
	profile, found := profilesCache[userID]
	if !found {
		log.Printf("[REPLY_VALIDATION] Chat %d: Профиль пользователя %d не найден в кеше, загружаем", chatID, userID)

		// Загружаем профиль из хранилища
		loadedProfile, err := store.GetUserProfile(chatID, userID)
		if err != nil {
			log.Printf("[REPLY_VALIDATION] Chat %d: Ошибка загрузки профиля для userID %d: %v", chatID, userID, err)
			return "", fmt.Errorf("failed to load profile for user %d: %w", userID, err)
		}

		if loadedProfile != nil {
			profilesCache[userID] = loadedProfile
			profile = loadedProfile
		}
	}

	// Определяем алиас пользователя с дисамбигуацией
	var userAlias string
	if globalBotInstance != nil && globalBotInstance.userValidator != nil {
		userAlias = globalBotInstance.userValidator.FormatUserWithDisambiguation(chatID, userID, "direct_reply", replyMsg)
	} else {
		// Fallback логика
		if profile != nil && profile.Alias != "" {
			userAlias = profile.Alias
		} else if replyMsg.From.FirstName != "" {
			userAlias = replyMsg.From.FirstName
		} else if replyMsg.From.UserName != "" {
			userAlias = replyMsg.From.UserName
		} else {
			userAlias = fmt.Sprintf("User_%d", userID)
		}

		// Добавляем ID для дисамбигуации
		userAlias = fmt.Sprintf("%s@U%d", userAlias, userID)
	}

	log.Printf("[REPLY_VALIDATION] Chat %d: Валидирована ссылка на сообщение %d от пользователя %s", chatID, replyMsg.MessageID, userAlias)

	return userAlias, nil
}

// fixReplyChainUserIdentification исправляет идентификацию пользователей в цепочке ответов
func fixReplyChainUserIdentification(chatID int64, replyChain []*tgbotapi.Message, store storage.ChatHistoryStorage) ([]*tgbotapi.Message, []string) {
	if len(replyChain) == 0 {
		return replyChain, nil
	}

	// Создаем кеш профилей для цепочки ответов
	profilesCache := make(map[int64]*storage.UserProfile)
	userIDs := make(map[int64]bool)

	// Собираем всех пользователей в цепочке
	for _, msg := range replyChain {
		if msg != nil && msg.From != nil {
			userIDs[msg.From.ID] = true
		}
	}

	// Предварительно загружаем профили
	for userID := range userIDs {
		profile, err := store.GetUserProfile(chatID, userID)
		if err != nil {
			log.Printf("[REPLY_CHAIN_FIX] Chat %d: Ошибка загрузки профиля для userID %d: %v", chatID, userID, err)
		} else if profile != nil {
			profilesCache[userID] = profile
		}
	}

	// Валидируем цепочку
	validatedChain, warnings := validateReplyChain(chatID, replyChain, store)

	// Дополнительно проверяем каждое сообщение в цепочке
	for _, msg := range validatedChain {
		if msg != nil && msg.From != nil {
			userID := msg.From.ID
			if _, found := profilesCache[userID]; !found {
				warnings = append(warnings, fmt.Sprintf("Пользователь %d не найден в профилях", userID))
			}
		}
	}

	log.Printf("[REPLY_CHAIN_FIX] Chat %d: Исправлена идентификация пользователей в цепочке. Предупреждений: %d",
		chatID, len(warnings))

	return validatedChain, warnings
}

// getPersonalityContext получает контекст личности для каузального анализатора
func (b *Bot) getPersonalityContext(chatID int64, promptType string) (string, error) {
	// Используем существующий метод buildPersonalityContext
	personalityContext := b.buildPersonalityContext(chatID, true, true)
	if personalityContext == "" {
		return "", fmt.Errorf("не удалось получить контекст личности для чата %d", chatID)
	}
	return personalityContext, nil
}

// getStyleInstructions получает инструкции стиля для каузального анализатора
func (b *Bot) getStyleInstructions() string {
	// Получаем инструкции из памяти личности для любого чата
	// Поскольку инструкции стиля общие для всех чатов, берем из первого найденного
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil || len(chatIDs) == 0 {
		return ""
	}

	// Пробуем получить инструкции из первого чата
	memory, err := b.storage.GetPersonalityMemory(chatIDs[0])
	if err != nil || memory == nil {
		return ""
	}

	return memory.StyleInstructions
}

// applyCausalInfluenceToContext применяет каузальные корректировки к контексту диалога
func (b *Bot) applyCausalInfluenceToContext(originalContext string, influence *BehavioralInfluenceResult) string {
	if influence == nil || len(influence.BehavioralAdjustments) == 0 {
		return originalContext
	}

	// Формируем дополнительный контекст на основе каузального влияния
	var causalContext strings.Builder
	causalContext.WriteString("\n\n=== КАУЗАЛЬНОЕ ВЛИЯНИЕ НА ПОВЕДЕНИЕ ===\n")

	// Добавляем поведенческие корректировки
	if len(influence.BehavioralAdjustments) > 0 {
		causalContext.WriteString("Корректировки поведения:\n")
		for _, adjustment := range influence.BehavioralAdjustments {
			causalContext.WriteString(fmt.Sprintf("- %s: %s (причина: %s)\n",
				adjustment.Aspect, adjustment.Adjustment, adjustment.Reason))
		}
	}

	// Добавляем активированные воспоминания
	if len(influence.TriggeredMemories) > 0 {
		causalContext.WriteString("\nАктивированные воспоминания:\n")
		for _, memory := range influence.TriggeredMemories {
			causalContext.WriteString(fmt.Sprintf("- %s\n", memory))
		}
	}

	// Добавляем общую стратегию
	if influence.OverallStrategy != "" {
		causalContext.WriteString(fmt.Sprintf("\nОбщая стратегия: %s\n", influence.OverallStrategy))
	}

	causalContext.WriteString("=== КОНЕЦ КАУЗАЛЬНОГО ВЛИЯНИЯ ===\n\n")

	// Возвращаем исходный контекст с добавленным каузальным влиянием
	return originalContext + causalContext.String()
}

// getEmotionalContextForUser получает эмоциональный контекст для конкретного пользователя
func (b *Bot) getEmotionalContextForUser(chatID, userID int64) string {
	if !b.config.EmotionalLearningEnabled {
		return ""
	}

	return b.GetEmotionalContext(chatID, userID)
}

// enrichPromptWithEmotionalContext обогащает промпт эмоциональным контекстом пользователя
func (b *Bot) enrichPromptWithEmotionalContext(basePrompt string, chatID, userID int64) string {
	emotionalContext := b.getEmotionalContextForUser(chatID, userID)
	if emotionalContext == "" {
		return basePrompt
	}

	// Проверяем, есть ли плейсхолдер для эмоционального контекста
	if strings.Contains(basePrompt, "{EMOTIONAL_CONTEXT}") {
		return strings.ReplaceAll(basePrompt, "{EMOTIONAL_CONTEXT}", emotionalContext)
	}

	// Если плейсхолдера нет, добавляем контекст в конец
	return basePrompt + "\n\n" + emotionalContext
}

// titleCase возвращает копию строки с первой буквой в верхнем регистре.
// Замена strings.Title (deprecated since Go 1.18).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}
