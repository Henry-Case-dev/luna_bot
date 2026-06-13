package bot

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ContextIsolationType определяет тип изоляции контекста
type ContextIsolationType string

const (
	// IsolationUserSpecific — ответ конкретному пользователю (reply_to / @username)
	// В контекст включается ТОЛЬКО профиль и история этого пользователя.
	IsolationUserSpecific ContextIsolationType = "user_specific"

	// IsolationGroupActive — ответ группе активных участников
	// Контекст: выжимка профилей активных участников (без детальных досье).
	IsolationGroupActive ContextIsolationType = "group_active"

	// IsolationGeneral — общий ответ в чат
	// Контекст: только саммари последних сообщений, без личных профилей.
	IsolationGeneral ContextIsolationType = "general"
)

// ContextIsolator управляет изоляцией контекста для групповых чатов
type ContextIsolator struct {
	bot *Bot
}

// NewContextIsolator создаёт новый изолятор контекста
func NewContextIsolator(bot *Bot) *ContextIsolator {
	return &ContextIsolator{bot: bot}
}

// IsolateContext изолирует контекст в зависимости от типа ответа.
// Принимает сырой контекст и возвращает отфильтрованный.
func (ci *ContextIsolator) IsolateContext(
	chatID int64,
	targetUserID int64,
	isolationType ContextIsolationType,
	rawContext string,
) string {
	if rawContext == "" {
		return rawContext
	}

	switch isolationType {
	case IsolationUserSpecific:
		if targetUserID != 0 {
			return ci.StripSensitiveDataXML(rawContext, targetUserID)
		}
		return rawContext

	case IsolationGroupActive:
		return ci.StripSensitiveDataXML(rawContext, -1)

	case IsolationGeneral:
		return ci.StripSensitiveDataXML(rawContext, 0)

	default:
		return rawContext
	}
}

// DetermineIsolationType определяет тип изоляции на основе решения free_will.
func (ci *ContextIsolator) DetermineIsolationType(
	chatID int64,
	decision *FreeWillShouldReplyDecision,
	targetMessageID int,
) ContextIsolationType {
	if decision == nil {
		return IsolationGeneral
	}

	switch decision.ReplyType {
	case "direct_reply":
		return IsolationUserSpecific
	case "take_response":
		return IsolationUserSpecific
	case "context_based":
		return IsolationGroupActive
	case "silence_response":
		return IsolationGeneral
	default:
		return IsolationGeneral
	}
}

// BuildUserSpecificContext строит контекст для ответа конкретному пользователю.
// Включает: профиль пользователя (alias, bio), его последние сообщения, reply chain.
func (ci *ContextIsolator) BuildUserSpecificContext(chatID int64, userID int64, targetMessageID int) string {
	if ci.bot == nil || ci.bot.storage == nil {
		return ""
	}

	// Получаем профиль целевого пользователя
	profile, err := ci.bot.storage.GetUserProfile(chatID, userID)
	if err != nil || profile == nil {
		log.Printf("[ContextIsolator] Не удалось получить профиль user %d в чате %d: %v", userID, chatID, err)
	}

	// Получаем общие сообщения
	messages, err := ci.bot.storage.GetMessages(chatID, ci.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[ContextIsolator] Ошибка получения сообщений: %v", err)
	}

	var result strings.Builder

	// Заголовок контекста
	result.WriteString(fmt.Sprintf("\n=== КОНТЕКСТ ДЛЯ ОТВЕТА ПОЛЬЗОВАТЕЛЮ ===\n"))
	result.WriteString(fmt.Sprintf("Целевой пользователь ID: %d\n", userID))

	// Профиль целевого пользователя
	if profile != nil {
		result.WriteString("\n--- ПРОФИЛЬ ПОЛЬЗОВАТЕЛЯ ---\n")
		if profile.Alias != "" {
			result.WriteString(fmt.Sprintf("Имя: %s\n", profile.Alias))
		}
		if profile.RealName != "" {
			result.WriteString(fmt.Sprintf("Настоящее имя: %s\n", profile.RealName))
		}
		if profile.Username != "" {
			result.WriteString(fmt.Sprintf("Юзернейм: @%s\n", profile.Username))
		}
		if profile.Bio != "" {
			result.WriteString(fmt.Sprintf("Bio: %s\n", profile.Bio))
		}
		if profile.AutoBio != "" {
			result.WriteString(fmt.Sprintf("AutoBio: %s\n", profile.AutoBio))
		}
		result.WriteString("--- КОНЕЦ ПРОФИЛЯ ---\n\n")
	}

	// Сообщения целевого пользователя
	if len(messages) > 0 {
		userMessages := make([]*tgbotapi.Message, 0)
		for _, msg := range messages {
			if msg.From != nil && int64(msg.From.ID) == userID {
				userMessages = append(userMessages, msg)
			}
		}

		limit := 20
		if len(userMessages) > limit {
			userMessages = userMessages[len(userMessages)-limit:]
		}

		if len(userMessages) > 0 {
			result.WriteString(fmt.Sprintf("--- ПОСЛЕДНИЕ %d СООБЩЕНИЙ ПОЛЬЗОВАТЕЛЯ ---\n", len(userMessages)))
			formatter := NewUnifiedMessageFormatter(ci.bot.storage, ci.bot.config.TimeZone)
			formatter.SetDisableUserProfiles(ci.bot.config.DisableUserProfiles)
			result.WriteString(formatter.FormatMessagesXML(chatID, userMessages))
			result.WriteString("--- КОНЕЦ СООБЩЕНИЙ ПОЛЬЗОВАТЕЛЯ ---\n\n")
		}
	}

	// Общий контекст (последние сообщения) — для понимания темы
	if len(messages) > 0 {
		limit := 15
		if len(messages) > limit {
			messages = messages[len(messages)-limit:]
		}
		result.WriteString("--- ОБЩИЙ КОНТЕКСТ ЧАТА (последние сообщения) ---\n")
		formatter := NewUnifiedMessageFormatter(ci.bot.storage, ci.bot.config.TimeZone)
		formatter.SetDisableUserProfiles(ci.bot.config.DisableUserProfiles)
		rawChatContext := formatter.FormatMessagesXML(chatID, messages)
		// Для общего контекста оставляем только alias, убираем bio/autobio чужих
		result.WriteString(ci.StripSensitiveDataXML(rawChatContext, userID))
		result.WriteString("--- КОНЕЦ ОБЩЕГО КОНТЕКСТА ---\n")
	}

	return result.String()
}

// BuildGroupActiveContext строит контекст для ответа группе.
// Включает: краткие профили активных участников (только alias), последние сообщения.
func (ci *ContextIsolator) BuildGroupActiveContext(chatID int64) string {
	if ci.bot == nil || ci.bot.storage == nil {
		return ""
	}

	messages, err := ci.bot.storage.GetMessages(chatID, ci.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[ContextIsolator] Ошибка получения сообщений: %v", err)
	}

	if len(messages) == 0 {
		return ""
	}

	// Собираем активных участников
	activeUsers := make(map[int64]*storage.UserProfile)
	for _, msg := range messages {
		if msg.From != nil {
			uid := int64(msg.From.ID)
			if _, exists := activeUsers[uid]; !exists {
				profile, err := ci.bot.storage.GetUserProfile(chatID, uid)
				if err == nil && profile != nil {
					activeUsers[uid] = profile
				}
			}
		}
	}

	var result strings.Builder
	result.WriteString("\n=== КОНТЕКСТ ДЛЯ ОТВЕТА ГРУППЕ ===\n")

	// Краткие профили активных участников (только alias)
	if len(activeUsers) > 0 {
		result.WriteString(fmt.Sprintf("\nАктивные участники (%d):\n", len(activeUsers)))
		for _, profile := range activeUsers {
			if profile.Alias != "" {
				result.WriteString(fmt.Sprintf("- %s\n", profile.Alias))
			} else if profile.Username != "" {
				result.WriteString(fmt.Sprintf("- @%s\n", profile.Username))
			}
		}
		result.WriteString("\n")
	}

	// Последние сообщения
	limit := 30
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	result.WriteString("--- ПОСЛЕДНИЕ СООБЩЕНИЯ ---\n")
	formatter := NewUnifiedMessageFormatter(ci.bot.storage, ci.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(ci.bot.config.DisableUserProfiles)
	rawContext := formatter.FormatMessagesXML(chatID, messages)
	// Убираем конфиденциальные данные, сохраняем alias
	result.WriteString(ci.StripSensitiveDataXML(rawContext, -1))
	result.WriteString("--- КОНЕЦ СООБЩЕНИЙ ---\n")

	return result.String()
}

// BuildGeneralContext строит общий контекст.
// Включает: последние N сообщений без привязки к профилям.
func (ci *ContextIsolator) BuildGeneralContext(chatID int64) string {
	if ci.bot == nil || ci.bot.storage == nil {
		return ""
	}

	messages, err := ci.bot.storage.GetMessages(chatID, ci.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[ContextIsolator] Ошибка получения сообщений: %v", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений"
	}

	limit := 25
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	formatter := NewUnifiedMessageFormatter(ci.bot.storage, ci.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(ci.bot.config.DisableUserProfiles)
	rawContext := formatter.FormatMessagesXML(chatID, messages)
	// Для общего контекста убираем все личные данные
	return ci.StripSensitiveDataXML(rawContext, 0)
}

// StripSensitiveData удаляет из контекста конфиденциальные данные (старый flat-string формат).
// keepUserID:
//
//	>0  — сохранять BIO/AUTOBIO только для этого пользователя
//	0   — удалять все BIO/AUTOBIO
//	-1  — сохранять alias, удалять BIO/AUTOBIO всех (групповой режим)
func (ci *ContextIsolator) StripSensitiveData(rawContext string, keepUserID int64) string {
	if rawContext == "" {
		return rawContext
	}

	result := rawContext

	// Удаляем секции [BIO]...[/BIO], [AUTOBIO]...[/AUTOBIO], [DOSSIER]...[/DOSSIER]
	result = regexp.MustCompile(`(?is)\[BIO\].*?\[/BIO\]`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?is)\[AUTOBIO\].*?\[/AUTOBIO\]`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?is)\[DOSSIER\].*?\[/DOSSIER\]`).ReplaceAllString(result, "")

	// Удаляем @U{id} паттерны (но не @username!)
	result = regexp.MustCompile(`@U\d+`).ReplaceAllString(result, "")

	if keepUserID > 0 {
		// Режим конкретного пользователя: разбиваем по блокам сообщений
		// и удаляем bio/autobio/username/real_name только для чужих блоков
		result = ci.stripNonTargetUserData(result, keepUserID)
	} else {
		// Общий/групповой режим: удаляем все bio/autobio/username/real_name
		result = regexp.MustCompile(`(?im)^\s*bio:\s*[^\n]*\n`).ReplaceAllString(result, "")
		result = regexp.MustCompile(`(?im)^\s*auto_bio:\s*[^\n]*\n`).ReplaceAllString(result, "")
		result = regexp.MustCompile(`(?im)^\s*username:\s*@[^\n]*\n`).ReplaceAllString(result, "")
		result = regexp.MustCompile(`(?im)^\s*real_name:\s*[^\n]*\n`).ReplaceAllString(result, "")
	}

	// Удаляем пустые строки-дубли (3+ подряд пустых строк -> 2)
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")

	return result
}

// IsolateMessages returns a ChatML message array for the given isolation type.
// Bot messages map to role "assistant", all other users map to role "user".
// Content is plain message text — no inline tags, no profiles, no metadata.
func (ci *ContextIsolator) IsolateMessages(chatID int64, targetUserID int64, isolationType ContextIsolationType, limit int) ([]llm.ChatMessage, error) {
	if ci.bot == nil || ci.bot.storage == nil {
		return nil, fmt.Errorf("bot or storage not available")
	}

	messages, err := ci.bot.storage.GetMessages(chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	if len(messages) == 0 {
		return nil, nil
	}

	formatter := NewUnifiedMessageFormatter(ci.bot.storage, ci.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(ci.bot.config.DisableUserProfiles)

	switch isolationType {
	case IsolationUserSpecific:
		if targetUserID != 0 {
			return ci.isolateUserSpecificMessages(messages, targetUserID, formatter, chatID), nil
		}
		return formatter.FormatChatMessages(chatID, messages), nil

	case IsolationGroupActive:
		return formatter.FormatChatMessages(chatID, messages), nil

	default:
		return formatter.FormatChatMessages(chatID, messages), nil
	}
}

func (ci *ContextIsolator) isolateUserSpecificMessages(messages []*tgbotapi.Message, targetUserID int64, formatter *UnifiedMessageFormatter, chatID int64) []llm.ChatMessage {
	userMessages := make([]*tgbotapi.Message, 0)
	for _, msg := range messages {
		if msg.From != nil && int64(msg.From.ID) == targetUserID {
			userMessages = append(userMessages, msg)
		}
	}

	limit := 20
	if len(userMessages) > limit {
		userMessages = userMessages[len(userMessages)-limit:]
	}

	generalLimit := 15
	startIdx := len(messages)
	if startIdx > generalLimit {
		startIdx = startIdx - generalLimit
	}
	generalMessages := messages[startIdx:]

	allMessages := make([]*tgbotapi.Message, 0, len(userMessages)+len(generalMessages))
	allMessages = append(allMessages, userMessages...)
	allMessages = append(allMessages, generalMessages...)

	return formatter.FormatChatMessages(chatID, allMessages)
}

// stripNonTargetUserData удаляет bio/autobio/username/real_name из блоков,
// принадлежащих нецелевым пользователям
func (ci *ContextIsolator) stripNonTargetUserData(text string, keepUserID int64) string {
	targetTag := fmt.Sprintf("[U%d:", keepUserID)

	// Ищем все блоки сообщений, разделённые по [MSG_ID:...]...[/MSG_ID:...]
	msgIDRe := regexp.MustCompile(`\[MSG_ID:\d+\]`)
	closeRe := regexp.MustCompile(`\[/MSG_ID:\d+\]`)

	// Разбиваем: ищем открывающие и закрывающие теги и обрабатываем сегменты
	var result strings.Builder
	remainder := text

	for {
		openLoc := msgIDRe.FindStringIndex(remainder)
		if openLoc == nil {
			// Больше нет блоков — дописываем остаток как есть
			result.WriteString(remainder)
			break
		}

		// Всё до открывающего тега пишем как есть
		result.WriteString(remainder[:openLoc[0]])

		// Ищем закрывающий тег
		closeLoc := closeRe.FindStringIndex(remainder[openLoc[1]:])
		if closeLoc == nil {
			// Нет закрывающего тега — пишем остаток как есть
			result.WriteString(remainder[openLoc[0]:])
			break
		}

		blockEnd := openLoc[1] + closeLoc[1]
		block := remainder[openLoc[0]:blockEnd]

		if strings.Contains(block, targetTag) {
			// Блок целевого пользователя — оставляем как есть
			result.WriteString(block)
		} else {
			// Блок чужого пользователя — удаляем конфиденциальные поля
			cleaned := block
			cleaned = regexp.MustCompile(`(?im)^\s*bio:\s*[^\n]*\n`).ReplaceAllString(cleaned, "")
			cleaned = regexp.MustCompile(`(?im)^\s*auto_bio:\s*[^\n]*\n`).ReplaceAllString(cleaned, "")
			cleaned = regexp.MustCompile(`(?im)^\s*username:\s*@[^\n]*\n`).ReplaceAllString(cleaned, "")
			cleaned = regexp.MustCompile(`(?im)^\s*real_name:\s*[^\n]*\n`).ReplaceAllString(cleaned, "")
			result.WriteString(cleaned)
		}

		remainder = remainder[blockEnd:]
		if len(remainder) == 0 {
			break
		}
	}

	return result.String()
}

// StripSensitiveDataXML удаляет конфиденциальные данные из XML-контекста.
// keepUserID:
//
//	>0 — сохранять BIO/REAL_NAME только для этого пользователя
//	0  — удалять все BIO/REAL_NAME
//	-1 — удалять BIO/REAL_NAME всех (групповой режим)
func (ci *ContextIsolator) StripSensitiveDataXML(rawContext string, keepUserID int64) string {
	if rawContext == "" {
		return rawContext
	}

	result := rawContext

	if keepUserID > 0 {
		result = ci.stripNonTargetBIOsXML(result, keepUserID)
	} else {
		re := regexp.MustCompile(`(?is)<BIO>.*?</BIO>\s*`)
		result = re.ReplaceAllString(result, "<BIO></BIO>")
		re = regexp.MustCompile(`(?is)<REAL_NAME>.*?</REAL_NAME>\s*`)
		result = re.ReplaceAllString(result, "<REAL_NAME></REAL_NAME>")
	}

	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")

	return result
}

func (ci *ContextIsolator) stripNonTargetBIOsXML(text string, keepUserID int64) string {
	targetIDStr := fmt.Sprintf("%d", keepUserID)

	msgRe := regexp.MustCompile(`(?s)<MSG\s+[^>]*ROLE="user"[^>]*>.*?</MSG>`)

	var result strings.Builder
	remainder := text

	for {
		loc := msgRe.FindStringIndex(remainder)
		if loc == nil {
			result.WriteString(remainder)
			break
		}

		result.WriteString(remainder[:loc[0]])
		block := remainder[loc[0]:loc[1]]

		if !strings.Contains(block, fmt.Sprintf(`<USER ID="%s"`, targetIDStr)) {
			re := regexp.MustCompile(`(?is)<BIO>.*?</BIO>\s*`)
			block = re.ReplaceAllString(block, "<BIO></BIO>")
			re = regexp.MustCompile(`(?is)<REAL_NAME>.*?</REAL_NAME>\s*`)
			block = re.ReplaceAllString(block, "<REAL_NAME></REAL_NAME>")
		}

		result.WriteString(block)
		remainder = remainder[loc[1]:]
	}

	return result.String()
}
