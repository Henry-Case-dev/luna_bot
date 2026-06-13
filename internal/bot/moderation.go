package bot

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	"github.com/Henry-Case-dev/luna_bot/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ModerationService обрабатывает логику модерации чатов
type ModerationService struct {
	bot             *Bot                          // Ссылка на основной объект бота
	messageCounters map[int64]int                 // Счетчик сообщений для каждого чата [chatID]count
	messageBuffer   map[int64][]*tgbotapi.Message // Буфер сообщений для каждого чата [chatID]messages
	// activePurges отслеживает активные задачи очистки [chatID][userID]cancelFunc
	activePurges map[int64]map[int64]context.CancelFunc
	rules        []config.ModerationRule // Загруженные правила модерации
	activeChats  map[int64]bool          // Чаты, в которых модерация активна [chatID]isActive
	mutex        sync.RWMutex            // Мьютекс для защиты доступа к картам
	purgeWG      sync.WaitGroup          // WaitGroup для ожидания завершения очистки

	// Новые поля для оптимизации
	deletedMessageCache map[string]time.Time // Кэш ID удаленных/несуществующих сообщений [chatID:messageID]timestamp
	cacheMutex          sync.RWMutex         // Мьютекс для кэша
}

// Константы для оптимизации
const (
	// Время жизни записи в кэше удаленных сообщений (1 час)
	deletedMessageCacheTTL = 1 * time.Hour
	// Интервал очистки кэша (15 минут)
	cacheCleanupInterval = 15 * time.Minute
	// Максимальный возраст сообщений для попытки удаления (48 часов - лимит Telegram)
	maxMessageAgeForDeletion = 48 * time.Hour
)

// NewModerationService создает новый экземпляр сервиса модерации
func NewModerationService(bot *Bot) *ModerationService {
	ms := &ModerationService{
		bot:                 bot,
		messageCounters:     make(map[int64]int),
		messageBuffer:       make(map[int64][]*tgbotapi.Message),
		activePurges:        make(map[int64]map[int64]context.CancelFunc),
		rules:               bot.config.ModRules, // Копируем правила из конфига
		activeChats:         make(map[int64]bool),
		mutex:               sync.RWMutex{},
		purgeWG:             sync.WaitGroup{},
		deletedMessageCache: make(map[string]time.Time),
		cacheMutex:          sync.RWMutex{},
	}

	// Запускаем фоновую очистку кэша
	go ms.startCacheCleanup()

	return ms
}

// startCacheCleanup запускает фоновую задачу очистки кэша устаревших записей
func (ms *ModerationService) startCacheCleanup() {
	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.cleanupExpiredCacheEntries()
		case <-ms.bot.stop:
			return
		}
	}
}

// cleanupExpiredCacheEntries удаляет устаревшие записи из кэша
func (ms *ModerationService) cleanupExpiredCacheEntries() {
	ms.cacheMutex.Lock()
	defer ms.cacheMutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, timestamp := range ms.deletedMessageCache {
		if now.Sub(timestamp) > deletedMessageCacheTTL {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(ms.deletedMessageCache, key)
	}

	if len(expiredKeys) > 0 && ms.bot.config.Debug {
		log.Printf("[DEBUG][Moderation Cache] Очищено %d устаревших записей из кэша удаленных сообщений", len(expiredKeys))
	}
}

// isMessageInDeletedCache проверяет, находится ли сообщение в кэше удаленных
func (ms *ModerationService) isMessageInDeletedCache(chatID int64, messageID int) bool {
	ms.cacheMutex.RLock()
	defer ms.cacheMutex.RUnlock()

	key := fmt.Sprintf("%d:%d", chatID, messageID)
	timestamp, exists := ms.deletedMessageCache[key]

	if !exists {
		return false
	}

	// Проверяем, не устарела ли запись
	if time.Since(timestamp) > deletedMessageCacheTTL {
		// Запланируем удаление при следующей очистке
		return false
	}

	return true
}

// addToDeletedCache добавляет сообщение в кэш удаленных
func (ms *ModerationService) addToDeletedCache(chatID int64, messageID int) {
	ms.cacheMutex.Lock()
	defer ms.cacheMutex.Unlock()

	key := fmt.Sprintf("%d:%d", chatID, messageID)
	ms.deletedMessageCache[key] = time.Now()

	if ms.bot.config.Debug {
		log.Printf("[DEBUG][Moderation Cache] Добавлено в кэш удаленных: %s", key)
	}
}

// isMessageTooOld проверяет, не слишком ли старое сообщение для удаления
func (ms *ModerationService) isMessageTooOld(message *tgbotapi.Message) bool {
	if message == nil || message.Date == 0 {
		return true
	}

	messageTime := time.Unix(int64(message.Date), 0)
	age := time.Since(messageTime)

	return age > maxMessageAgeForDeletion
}

// deleteMessageOptimized оптимизированная версия удаления сообщения с кэшированием
func (ms *ModerationService) deleteMessageOptimized(chatID int64, messageID int, messageTime time.Time) bool {
	// Проверяем кэш удаленных сообщений
	if ms.isMessageInDeletedCache(chatID, messageID) {
		if ms.bot.config.Debug {
			log.Printf("[DEBUG][Moderation Cache] Сообщение %d в чате %d уже в кэше удаленных, пропуск", messageID, chatID)
		}
		return false
	}

	// Проверяем возраст сообщения
	age := time.Since(messageTime)
	if age > maxMessageAgeForDeletion {
		if ms.bot.config.Debug {
			log.Printf("[DEBUG][Moderation Cache] Сообщение %d в чате %d слишком старое (%v), пропуск", messageID, chatID, age)
		}
		ms.addToDeletedCache(chatID, messageID) // Добавляем в кэш, чтобы не пытаться снова
		return false
	}

	// Пытаемся удалить сообщение
	deleted := ms.bot.deleteMessageSilent(chatID, messageID)

	// Если удаление не удалось (сообщение не найдено), добавляем в кэш
	if !deleted {
		ms.addToDeletedCache(chatID, messageID)
	}

	return deleted
}

// CheckAdminRightsAndActivate проверяет права администратора бота в чате
// и активирует/деактивирует модерацию для этого чата.
func (ms *ModerationService) CheckAdminRightsAndActivate(chatID int64) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	log.Printf("[DEBUG][Moderation] CheckAdminRightsAndActivate CALLED for chat %d. MOD_ENABLED=%v, MOD_CHECK_ADMIN_RIGHTS=%v", chatID, ms.bot.config.ModEnabled, ms.bot.config.ModCheckAdminRights)

	// НОВАЯ ПРОВЕРКА: Если модерация глобально отключена, не активируем
	if !ms.bot.config.ModEnabled {
		log.Printf("[INFO][Moderation] Чат %d: Модерация ГЛОБАЛЬНО ОТКЛЮЧЕНА (MOD_ENABLED=false). Не активируем.", chatID)
		ms.activeChats[chatID] = false
		return
	}

	if isActive, exists := ms.activeChats[chatID]; exists && isActive {
		log.Printf("[DEBUG][Moderation] Chat %d already known and active.", chatID)
		return
	}

	// Используем существующий метод бота для получения информации о члене чата
	botMember, err := ms.bot.getBotMember(chatID) // ИСПРАВЛЕНО: используем ms.bot.getBotMember
	if err != nil {
		log.Printf("[ERROR][Moderation] Чат %d: Не удалось получить информацию о боте: %v. Модерация НЕ активирована.", chatID, err)
		ms.activeChats[chatID] = false
		return
	}

	log.Printf("[DEBUG][Moderation] Chat %d: Bot member status: %s", chatID, botMember.Status)

	canActivate := false
	reason := ""

	if !ms.bot.config.ModCheckAdminRights {
		canActivate = true
		reason = "проверка прав администратора отключена"
	} else {
		if botMember.Status == "administrator" || botMember.Status == "creator" {
			if botMember.Status == "administrator" {
				// Проверяем права администратора напрямую из структуры ChatMember
				if !botMember.CanRestrictMembers || !botMember.CanDeleteMessages {
					log.Printf("[INFO][Moderation] Чат %d: Бот является администратором, но не имеет прав на ограничение участников или удаление сообщений. Модерация НЕ активирована.", chatID)
					ms.bot.sendSystemMessage(chatID, "⚠️ Модерация не может быть включена: у бота нет прав на ограничение участников и удаление сообщений в этом чате.")
					ms.activeChats[chatID] = false
					return
				}
				canActivate = true
				reason = "бот является администратором с необходимыми правами"
			} else { // creator
				canActivate = true
				reason = "бот является создателем чата"
			}
		} else {
			log.Printf("[INFO][Moderation] Чат %d: Бот не является администратором или создателем (статус: %s). Модерация НЕ активирована.", chatID, botMember.Status)
			if ms.bot.config.ModCheckAdminRights {
				ms.bot.sendSystemMessage(chatID, fmt.Sprintf("⚠️ Модерация не может быть включена: бот не является администратором в этом чате (текущий статус: %s). Либо отключите проверку прав администратора (/togglesettings -> Модерация).", botMember.Status))
			}
			ms.activeChats[chatID] = false
			return
		}
	}

	if canActivate {
		ms.activeChats[chatID] = true
		log.Printf("[INFO][Moderation] Чат %d: Модерация АКТИВИРОВАНА. Причина: %s.", chatID, reason)

		// Отправляем уведомление - УБРАНО: теперь используется централизованное стартовое сообщение
		// notificationText := ""
		// if !ms.bot.config.ModCheckAdminRights {
		// 	notificationText = "✅ Модерация включена (проверка прав администратора отключена)."
		// } else {
		// 	notificationText = "✅ Модерация включена."
		// }
		// ms.bot.sendSystemMessage(chatID, notificationText) // ИСПРАВЛЕНО: используем sendSystemMessage (НЕ сохраняется в БД)

	} else {
		ms.activeChats[chatID] = false
		log.Printf("[INFO][Moderation] Чат %d: Модерация НЕ активирована (финальная проверка).", chatID)
	}
}

// ProcessIncomingMessage обрабатывает входящее сообщение для модерации.
// Увеличивает счетчик, добавляет в буфер и запускает проверку, если достигнут интервал.
func (ms *ModerationService) ProcessIncomingMessage(message *tgbotapi.Message) {
	log.Printf("[DEBUG][Moderation] ProcessIncomingMessage CALLED for chat %d, user %d (%s), message ID %d, text: \"%s\"", message.Chat.ID, message.From.ID, message.From.UserName, message.MessageID, message.Text)

	ms.mutex.RLock()
	isActive, chatKnown := ms.activeChats[message.Chat.ID]
	initialActiveStatusLog := fmt.Sprintf("[DEBUG][Moderation] Initial active status for chat %d: known=%v, active=%v", message.Chat.ID, chatKnown, isActive)
	ms.mutex.RUnlock()
	log.Println(initialActiveStatusLog)

	if !chatKnown {
		// Это может произойти, если бот был перезапущен и еще не получил "chat member update"
		// или если первое сообщение в чате пришло до того, как бот успел его обработать.
		go ms.CheckAdminRightsAndActivate(message.Chat.ID) // Запускаем в горутине, чтобы не блокировать
		log.Printf("[DEBUG][Moderation] Chat %d was not known. Called CheckAdminRightsAndActivate. Waiting 500ms.", message.Chat.ID)
		// Ждем немного, чтобы активация успела произойти (если это был первый вызов)
		time.Sleep(500 * time.Millisecond)
		ms.mutex.RLock()
		isActive, chatKnown = ms.activeChats[message.Chat.ID]
		finalActiveStatusLog := fmt.Sprintf("[DEBUG][Moderation] Final active status for chat %d after re-check: known=%v, active=%v", message.Chat.ID, chatKnown, isActive)
		ms.mutex.RUnlock()
		log.Println(finalActiveStatusLog)
	}

	if !isActive {
		if ms.bot.config.Debug {
			ms.mutex.RLock()
			currentActiveChats := make(map[int64]bool)
			for k, v := range ms.activeChats {
				currentActiveChats[k] = v
			}
			ms.mutex.RUnlock()
			log.Printf("[DEBUG][Moderation] Moderation for chat %d is NOT active. Skipping message. Current activeChats map: %v", message.Chat.ID, currentActiveChats)
		}
		return
	}
	log.Printf("[DEBUG][Moderation] Moderation for chat %d IS active. Proceeding with message processing.", message.Chat.ID)

	// Добавляем сообщение в буфер чата
	ms.mutex.Lock()
	ms.messageCounters[message.Chat.ID]++
	currentCount := ms.messageCounters[message.Chat.ID]
	ms.messageBuffer[message.Chat.ID] = append(ms.messageBuffer[message.Chat.ID], message)

	if ms.bot.config.Debug {
		log.Printf("[Moderation Process DEBUG] Чат %d: Сообщение ID %d добавлено в буфер. Счетчик: %d/%d", message.Chat.ID, message.MessageID, currentCount, ms.bot.config.ModInterval)
	}

	// Проверяем, достигнут ли интервал
	if currentCount >= ms.bot.config.ModInterval {
		log.Printf("[Moderation Process INFO] Чат %d: Достигнут интервал проверки (%d). Запуск анализа пакета сообщений.", message.Chat.ID, ms.bot.config.ModInterval)
		// Копируем буфер, чтобы избежать гонки данных при асинхронной обработке
		messagesToProcess := make([]*tgbotapi.Message, len(ms.messageBuffer[message.Chat.ID]))
		copy(messagesToProcess, ms.messageBuffer[message.Chat.ID])

		// Сбрасываем счетчик и очищаем буфер
		ms.messageCounters[message.Chat.ID] = 0
		ms.messageBuffer[message.Chat.ID] = make([]*tgbotapi.Message, 0, ms.bot.config.ModInterval)

		// Разблокируем мьютекс ПЕРЕД запуском горутины
		ms.mutex.Unlock()

		// Запускаем обработку пакета в отдельной горутине
		go ms.processMessageBatch(message.Chat.ID, messagesToProcess)
	} else {
		// Интервал не достигнут, просто разблокируем мьютекс
		ms.mutex.Unlock()
	}
}

// processMessageBatch обрабатывает пакет сообщений, проверяя их на соответствие правилам.
func (ms *ModerationService) processMessageBatch(chatID int64, messages []*tgbotapi.Message) {
	if ms.bot.config.Debug {
		log.Printf("[Moderation Batch DEBUG] Чат %d: Начало обработки пакета из %d сообщений.", chatID, len(messages))
	}

	// Получаем все профили пользователей для этого чата ОДИН РАЗ
	profiles, err := ms.bot.storage.GetAllUserProfiles(chatID)
	if err != nil {
		log.Printf("[Moderation Batch ERROR] Чат %d: Не удалось получить профили пользователей: %v. LLM контекст будет без псевдонимов.", chatID, err)
		profiles = []*storage.UserProfile{} // Используем пустой список в случае ошибки
	}
	profileMap := make(map[int64]*storage.UserProfile)
	for _, p := range profiles {
		profileMap[p.UserID] = p
	}

	// Используем новый унифицированный форматтер для контекста LLM
	formatter := NewUnifiedMessageFormatter(ms.bot.storage, ms.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(ms.bot.config.DisableUserProfiles)
	contextForLLM := formatter.FormatMessagesXML(chatID, messages)

	log.Printf("[Moderation] Chat %d: Использован унифицированный форматтер для %d сообщений", chatID, len(messages))

messageLoop:
	for _, msg := range messages {
		if msg == nil || msg.From == nil { // Пропускаем системные сообщения или сообщения без автора
			continue
		}

		userID := msg.From.ID
		messageText := msg.Text // Используем текст или подпись
		if messageText == "" {
			messageText = msg.Caption
		}
		// Не пропускаем сообщения без текста/подписи здесь,
		// так как правило "Любые" для конкретного юзера должно сработать даже на стикер/фото

		for _, rule := range ms.rules {
			// 1. Проверка соответствия Chat ID
			if rule.ParsedChatID != 0 && rule.ParsedChatID != chatID && rule.ParsedChatID != -1 {
				continue // Правило не для этого чата
			}

			// 2. Проверка соответствия User ID
			isUserSpecificRule := rule.ParsedUserID != 0 && rule.ParsedUserID != -1
			if isUserSpecificRule && rule.ParsedUserID != userID {
				continue // Правило для другого пользователя
			}

			// 3. Проверка ключевых слов
			// Если правило для конкретного пользователя и ключевые слова "Любые",
			// то совпадение по ключевым словам гарантировано для этого пользователя.
			// В остальных случаях проверяем ключевые слова.
			keywordsMatch := false
			if isUserSpecificRule && len(rule.Keywords) == 1 && strings.ToLower(rule.Keywords[0]) == "любые" {
				keywordsMatch = true
			} else {
				if messageText == "" && !(len(rule.Keywords) == 1 && strings.ToLower(rule.Keywords[0]) == "любые") {
					// Если текст пустой, и ключевые слова не "Любые", то нет совпадения
					// (кроме случая, когда правило user-specific и keywords "Любые", это обработано выше)
					continue
				}
				keywordsMatch = ms.matchKeywords(messageText, rule.Keywords)
			}

			if !keywordsMatch {
				continue // Ключевые слова не найдены или не применимы
			}

			// --- Правило сработало ---
			log.Printf("[Moderation Trigger INFO] Чат %d: Сообщение ID %d от пользователя %d (@%s) попало под правило '%s'.",
				chatID, msg.MessageID, userID, msg.From.UserName, rule.RuleName)

			// 4. Проверка LLM (если требуется)
			applyPunishment := false
			if rule.LLMInstruction == "none" {
				log.Printf("[Moderation Trigger DEBUG] Чат %d, Правило '%s': LLM проверка пропущена (instruction='none'). Наказание будет применено.", chatID, rule.RuleName)
				applyPunishment = true
			} else {
				log.Printf("[Moderation Trigger DEBUG] Чат %d, Правило '%s': Требуется проверка LLM.", chatID, rule.RuleName)
				// Вызываем LLM с отформатированным контекстом и инструкцией из правила
				llmVerdict, llmError := ms.bot.llm.GenerateResponseByType(llm.ResponseTypeModeration, rule.LLMInstruction, contextForLLM, float32(ms.bot.config.GeminiTemperatureSerious))

				if llmError != nil {
					log.Printf("[Moderation LLM ERROR] Чат %d, Правило '%s': Ошибка при вызове LLM: %v", chatID, rule.RuleName, llmError)
					// Не применяем наказание при ошибке LLM
				} else {
					// Очищаем ответ от возможных метаданных перед анализом
					llmVerdict = cleanupLLMResponse(llmVerdict)

					if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(llmVerdict)), "ПОЛОЖИТЕЛЬНО") {
						log.Printf("[Moderation LLM INFO] Чат %d, Правило '%s': LLM вернул ПОЛОЖИТЕЛЬНЫЙ вердикт. Наказание будет применено.", chatID, rule.RuleName)
						applyPunishment = true
					} else {
						log.Printf("[Moderation LLM INFO] Чат %d, Правило '%s': LLM вернул ОТРИЦАТЕЛЬНЫЙ вердикт. Наказание не применяется.", chatID, rule.RuleName)
					}
				}
			}

			// 5. Применение наказания (если требуется)
			if applyPunishment {
				ms.applyPunishment(chatID, userID, msg.From.UserName, rule, msg)
			}

			// Прерываем проверку ПРАВИЛ для ТЕКУЩЕГО сообщения, так как одно правило уже сработало
			continue messageLoop
		}
	}

	if ms.bot.config.Debug {
		log.Printf("[Moderation Batch DEBUG] Чат %d: Завершена обработка пакета из %d сообщений.", chatID, len(messages))
	}
}

// matchKeywords проверяет, содержит ли текст хотя бы одно из ключевых слов/фраз.
// Проверка выполняется без учета регистра, ищет целые слова/фразы.
func (ms *ModerationService) matchKeywords(text string, keywords []string) bool {
	if len(keywords) == 0 {
		return false // Нет ключевых слов для проверки
	}
	// Эта проверка теперь обрабатывается выше, в processMessageBatch,
	// в связке с user_id. Оставляем ее здесь для общих случаев, когда user_id не указан.
	if len(keywords) == 1 && strings.ToLower(keywords[0]) == "любые" {
		return true // Срабатывает всегда, если не user-specific правило
	}

	if text == "" { // Если текст пустой, то ключевые слова (кроме "Любые") не могут совпасть
		return false
	}

	lowerText := " " + strings.ToLower(text) + " " // Добавляем пробелы для поиска целых слов

	for _, keyword := range keywords {
		lowerKeyword := strings.ToLower(strings.TrimSpace(keyword))
		if lowerKeyword == "" {
			continue // Пропускаем пустые ключевые слова
		}

		// Ищем ключевое слово/фразу, окруженную не-буквенно-цифровыми символами или пробелами
		// Простой вариант: ищем " keyword " (с пробелами)
		if strings.Contains(lowerText, " "+lowerKeyword+" ") {
			if ms.bot.config.Debug {
				log.Printf("[Moderation Keyword DEBUG] Найдено ключевое слово '%s' в тексте: %s...", keyword, utils.TruncateString(text, 50))
			}
			return true
		}

		// TODO: Добавить более продвинутый поиск похожих слов (например, расстояние Левенштейна), если требуется.
		// Сейчас реализован только поиск точного совпадения целого слова/фразы.
	}

	return false // Ни одно ключевое слово не найдено
}

// applyPunishment применяет указанное наказание к пользователю.
func (ms *ModerationService) applyPunishment(chatID int64, userID int64, username string, rule config.ModerationRule, triggerMessage *tgbotapi.Message) {
	logPrefix := fmt.Sprintf("[Moderation Apply] Чат %d, Правило '%s', Пользователь %d (@%s):", chatID, rule.RuleName, userID, username)

	// 1. Определяем длительность наказания (для mute/ban)
	var untilDate int64 = 0 // 0 означает навсегда или не применимо
	switch rule.Punishment {
	case config.PunishMute:
		if ms.bot.config.ModMuteTimeMin > 0 {
			untilDate = time.Now().Add(time.Duration(ms.bot.config.ModMuteTimeMin) * time.Minute).Unix()
		} else {
			// Для перманентного мута устанавливаем дату через 366 дней (требование Telegram API)
			untilDate = time.Now().AddDate(1, 0, 1).Unix()
		}
	case config.PunishBan:
		if ms.bot.config.ModBanTimeMin > 0 {
			untilDate = time.Now().Add(time.Duration(ms.bot.config.ModBanTimeMin) * time.Minute).Unix()
		} else {
			// Для перманентного бана устанавливаем дату через 366 дней (требование Telegram API)
			untilDate = time.Now().AddDate(1, 0, 1).Unix()
		}
	}

	// 2. Применяем наказание через Telegram API
	success := false
	var apiErr error
	switch rule.Punishment {
	case config.PunishMute:
		restrictConfig := tgbotapi.RestrictChatMemberConfig{
			ChatMemberConfig: tgbotapi.ChatMemberConfig{
				ChatID: chatID,
				UserID: userID,
			},
			UntilDate: untilDate,
			Permissions: &tgbotapi.ChatPermissions{ // Запрещаем всё, кроме чтения
				CanSendMessages:       false,
				CanSendMediaMessages:  false,
				CanSendPolls:          false,
				CanSendOtherMessages:  false,
				CanAddWebPagePreviews: false,
				CanChangeInfo:         false,
				CanInviteUsers:        false,
				CanPinMessages:        false,
			},
		}
		_, apiErr = ms.bot.api.Request(restrictConfig)
		if apiErr == nil {
			success = true
			if ms.bot.config.ModMuteTimeMin > 0 {
				log.Printf("%s Применен MUTE (до %v).", logPrefix, time.Unix(untilDate, 0))
			} else {
				log.Printf("%s Применен ПЕРМАНЕНТНЫЙ MUTE (до %v).", logPrefix, time.Unix(untilDate, 0))
			}
		}

	case config.PunishKick:
		var kickUntilDate int64
		if ms.bot.config.ModKickTimeMin > 0 {
			kickUntilDate = time.Now().Add(time.Duration(ms.bot.config.ModKickTimeMin) * time.Minute).Unix()
		} else {
			// Для перманентного кика устанавливаем дату через 366 дней (требование Telegram API)
			kickUntilDate = time.Now().AddDate(1, 0, 1).Unix()
		}
		banConfig := tgbotapi.BanChatMemberConfig{
			ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
			UntilDate:        kickUntilDate,
			RevokeMessages:   false, // Не удаляем сообщения при кике
		}
		_, apiErr = ms.bot.api.Request(banConfig)
		if apiErr == nil {
			success = true
			if ms.bot.config.ModKickTimeMin > 0 {
				log.Printf("%s Применен KICK (до %v).", logPrefix, time.Unix(kickUntilDate, 0))
			} else {
				log.Printf("%s Применен ПЕРМАНЕНТНЫЙ KICK (до %v).", logPrefix, time.Unix(kickUntilDate, 0))
			}
		}

	case config.PunishBan:
		banConfig := tgbotapi.BanChatMemberConfig{
			ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
			UntilDate:        untilDate,
			RevokeMessages:   false, // Пока не удаляем сообщения при бане, можно сделать опцией
		}
		_, apiErr = ms.bot.api.Request(banConfig)
		if apiErr == nil {
			success = true
			if ms.bot.config.ModBanTimeMin > 0 {
				log.Printf("%s Применен BAN (до %v).", logPrefix, time.Unix(untilDate, 0))
			} else {
				log.Printf("%s Применен ПЕРМАНЕНТНЫЙ BAN (до %v).", logPrefix, time.Unix(untilDate, 0))
			}
		}

	case config.PunishPurge:
		// Удаляем существующую задачу очистки для этого пользователя, если она есть
		ms.StopPurge(chatID, userID)

		purgeWindow := ms.bot.config.ModPurgeDuration
		// Проверка на nil или нулевую длительность, если необходимо (хотя envDefault должен это покрывать)
		if purgeWindow <= 0 {
			log.Printf("%s ModPurgeDuration некорректна или не задана ('%v'). Используется значение по умолчанию 1h.", logPrefix, ms.bot.config.ModPurgeDuration)
			purgeWindow = time.Hour
		}

		// Запускаем задачу очистки в отдельной горутине
		// Передаем ID триггерного сообщения
		ms.purgeWG.Add(1)
		go ms.purgeUserMessages(context.Background(), chatID, userID, purgeWindow, rule.RuleName, time.Unix(int64(triggerMessage.Date), 0), triggerMessage.MessageID)
		success = true // Сам факт запуска горутины считаем успехом применения наказания

	case config.PunishEdit:
		// EDIT: удаляем исходное сообщение и отправляем замену
		triggerTime := time.Unix(int64(triggerMessage.Date), 0)
		deleted := ms.deleteMessageOptimized(chatID, triggerMessage.MessageID, triggerTime)
		if deleted && rule.ReplacementText != "" {
			msg := tgbotapi.NewMessage(chatID, rule.ReplacementText)
			_, apiErr = ms.bot.api.Send(msg)
			if apiErr == nil {
				success = true
				log.Printf("%s Применен EDIT: сообщение заменено на '%s'.", logPrefix, rule.ReplacementText)
			}
		} else if deleted {
			success = true
		}

	case config.PunishNone:
		log.Printf("%s Тип наказания 'none', никаких действий не предпринято.", logPrefix)
		success = true // Действий не было, но и ошибки нет

	default:
		log.Printf("%s Неизвестный тип наказания: %s", logPrefix, rule.Punishment)
		apiErr = fmt.Errorf("неизвестный тип наказания: %s", rule.Punishment)
	}

	// 3. Обработка ошибок API
	if apiErr != nil {
		log.Printf("%s Ошибка применения наказания (%s) через API: %v", logPrefix, rule.Punishment, apiErr)
		// Отправляем сообщение об ошибке в чат (с автоудалением)
		notifyText := fmt.Sprintf("⚠️ Ошибка модерации (правило '%s'): Не удалось применить наказание '%s' к @%s. Ошибка: %v",
			rule.RuleName, rule.Punishment, username, apiErr)
		ms.bot.sendSystemMessage(chatID, notifyText) // НЕ сохраняется в БД
		return                                       // Прерываем выполнение, если наказание не удалось применить
	}

	// 4. Уведомления (если наказание успешно применено или тип 'none')
	if success {
		notifyEnabled := ms.bot.config.ModDefaultNotify
		// Переопределение стандартной настройки
		// (В текущей структуре rule нет поля для переопределения, используем Chat/User Notify)

		punishmentDurationStr := ""
		if untilDate > 0 {
			punishmentDurationStr = fmt.Sprintf(" до %s", time.Unix(untilDate, 0).Format("2006-01-02 15:04:05 MST"))
		}

		baseNotifyText := fmt.Sprintf("Модерация: К @%s применено наказание '%s'%s по правилу '%s'.",
			username, rule.Punishment, punishmentDurationStr, rule.RuleName)
		if rule.Punishment == config.PunishEdit {
			baseNotifyText = fmt.Sprintf("Модерация: Сообщение пользователя @%s заменено по правилу '%s'.", username, rule.RuleName)
		}
		if rule.Punishment == config.PunishKick {
			baseNotifyText = fmt.Sprintf("Модерация: @%s кикнут по правилу '%s'.", username, rule.RuleName)
		}
		if rule.Punishment == config.PunishPurge {
			baseNotifyText = fmt.Sprintf("Модерация: Запущена очистка сообщений @%s по правилу '%s'.", username, rule.RuleName)
		}
		if rule.Punishment == config.PunishNone {
			baseNotifyText = fmt.Sprintf("Модерация: Зафиксировано нарушение правила '%s' пользователем @%s.", rule.RuleName, username)
		}

		note := ""
		if rule.PunishmentNote != "" {
			note = fmt.Sprintf("\nПримечание: %s", rule.PunishmentNote)
		}

		// Уведомление в чат
		if rule.NotifyChat && notifyEnabled {
			ms.bot.sendSystemMessage(chatID, baseNotifyText+note) // НЕ сохраняется в БД
		}

		// Уведомление пользователя в ЛС
		if rule.NotifyUser && notifyEnabled {
			// Отправляем сообщение в ЛС пользователю userID
			// Убедимся, что не отправляем ему его же сообщение (если purge)
			if rule.Punishment != config.PunishPurge {
				pmText := fmt.Sprintf("В чате (ID: %d) к вам применено наказание '%s'%s по правилу '%s'.%s",
					chatID, rule.Punishment, punishmentDurationStr, rule.RuleName, note)
				ms.bot.sendReply(userID, pmText) // Отправляем в ЛС
			}
		}
	}
}

// purgeUserMessages удаляет сообщения пользователя за указанный период и само триггерное сообщение.
// Выполняется в отдельной горутине.
func (ms *ModerationService) purgeUserMessages(ctx context.Context, chatID int64, userID int64, duration time.Duration, ruleName string, triggerTime time.Time, triggerMessageID int) {
	defer ms.purgeWG.Done()
	ms.mutex.Lock()
	if _, ok := ms.activePurges[chatID]; !ok {
		ms.activePurges[chatID] = make(map[int64]context.CancelFunc)
	}
	ctx, cancel := context.WithCancel(ctx)
	ms.activePurges[chatID][userID] = cancel
	ms.mutex.Unlock()

	defer func() {
		ms.mutex.Lock()
		delete(ms.activePurges[chatID], userID)
		if len(ms.activePurges[chatID]) == 0 {
			delete(ms.activePurges, chatID)
		}
		ms.mutex.Unlock()
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERED][Moderation Purge] Чат %d, Пользователь %d: %v\n%s", chatID, userID, r, string(debug.Stack()))
		}
	}()

	log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d (триггер @ %s, ID сообщения %d): Начало очистки сообщений.",
		chatID, ruleName, userID, triggerTime.Format("15:04:05"), triggerMessageID)

	// Применяем задержку перед началом очистки, если настроено
	if ms.bot.config.ModPurgeDelay > 0 {
		log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d: Ожидание %s перед началом очистки.",
			chatID, ruleName, userID, ms.bot.config.ModPurgeDelay)

		select {
		case <-time.After(ms.bot.config.ModPurgeDelay):
			log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d: Задержка завершена, начинаем очистку.",
				chatID, ruleName, userID)
		case <-ctx.Done():
			log.Printf("[Moderation Purge CANCELED] Чат %d, Правило '%s', Пользователь %d: Операция отменена во время ожидания задержки.",
				chatID, ruleName, userID)
			return
		}
	}

	// Определяем окно времени для удаления
	// Сообщения, отправленные ПОСЛЕ triggerTime - duration и ДО triggerTime (или немного после, для захвата триггера)
	// untilTime := triggerTime.Add(1 * time.Second) // Немного запаса, чтобы захватить триггерное сообщение, если оно точно в triggerTime
	// sinceTime := triggerTime.Add(-duration)
	// Более точный подход: untilTime - это время триггера, sinceTime - это triggerTime минус duration
	untilTime := triggerTime
	sinceTime := triggerTime.Add(-duration)

	if ms.bot.config.Debug {
		log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d (триггер @ %s): Окно удаления: с %s по %s (длительность %s)",
			chatID, ruleName, userID, triggerTime.Format("15:04:05"),
			sinceTime.Format("15:04:05"), untilTime.Format("15:04:05"), duration.String())
	}

	messagesToDelete, err := ms.bot.storage.GetMessagesInRange(ctx, chatID, userID, sinceTime, untilTime, 0)
	if err != nil {
		log.Printf("[Moderation Purge ERROR] Чат %d, Правило '%s', Пользователь %d: Ошибка получения сообщений для удаления: %v", chatID, ruleName, userID, err)
		return
	}

	deletedCount := 0
	var lastErr error

	if len(messagesToDelete) > 0 {
		log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d (триггер @ %s): Найдено %d сообщений для удаления.",
			chatID, ruleName, userID, triggerTime.Format("15:04:05"), len(messagesToDelete))

		// Фильтруем сообщения по возрасту и кэшу перед попыткой удаления
		var filteredMessages []*tgbotapi.Message
		skippedOld := 0
		skippedCached := 0

		for _, msg := range messagesToDelete {
			if msg.From != nil && msg.From.ID == userID {
				// Проверяем кэш удаленных сообщений
				if ms.isMessageInDeletedCache(chatID, msg.MessageID) {
					skippedCached++
					continue
				}

				// Проверяем возраст сообщения
				if ms.isMessageTooOld(msg) {
					skippedOld++
					ms.addToDeletedCache(chatID, msg.MessageID) // Добавляем старые сообщения в кэш
					continue
				}

				filteredMessages = append(filteredMessages, msg)
			}
		}

		if skippedOld > 0 || skippedCached > 0 {
			log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d: Пропущено %d старых и %d кэшированных сообщений",
				chatID, ruleName, userID, skippedOld, skippedCached)
		}

		// Удаляем только отфильтрованные сообщения
		for _, msg := range filteredMessages {
			if ctx.Err() != nil {
				log.Printf("[Moderation Purge CANCELED] Чат %d, Правило '%s', Пользователь %d: Операция отменена.", chatID, ruleName, userID)
				return
			}

			// Используем оптимизированную функцию удаления
			messageTime := time.Unix(int64(msg.Date), 0)
			if ms.deleteMessageOptimized(chatID, msg.MessageID, messageTime) {
				deletedCount++
			}

			// Небольшая задержка, чтобы не зафлудить API Telegram
			time.Sleep(200 * time.Millisecond)
		}
	} else {
		log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d (триггер @ %s): Не найдено сообщений для удаления за период %s.",
			chatID, ruleName, userID, triggerTime.Format("15:04:05"), duration.String())
	}

	// Дополнительно удаляем само триггерное сообщение, если оно еще не было удалено
	// (могло быть удалено, если попало в окно untilTime)
	deletedTrigger := false
	isTriggerInList := false
	for _, msg := range messagesToDelete {
		if msg.MessageID == triggerMessageID {
			isTriggerInList = true
			break
		}
	}

	if !isTriggerInList {
		if ctx.Err() != nil {
			log.Printf("[Moderation Purge CANCELED] Чат %d, Правило '%s', Пользователь %d: Операция отменена (перед удалением триггера).", chatID, ruleName, userID)
			return
		}

		// Проверяем кэш и возраст триггерного сообщения
		if !ms.isMessageInDeletedCache(chatID, triggerMessageID) {
			triggerAge := time.Since(triggerTime)
			if triggerAge <= maxMessageAgeForDeletion {
				if ms.bot.config.Debug {
					log.Printf("[Moderation Purge DEBUG] Чат %d, Правило '%s', Пользователь %d: Попытка удалить триггерное сообщение ID %d отдельно.", chatID, ruleName, userID, triggerMessageID)
				}

				// Используем оптимизированную функцию удаления
				if ms.deleteMessageOptimized(chatID, triggerMessageID, triggerTime) {
					deletedTrigger = true
					if ms.bot.config.Debug {
						log.Printf("[Moderation Purge DEBUG] Чат %d, Правило '%s', Пользователь %d: Успешно удалено триггерное сообщение ID %d.", chatID, ruleName, userID, triggerMessageID)
					}
				}
			} else {
				if ms.bot.config.Debug {
					log.Printf("[Moderation Purge DEBUG] Чат %d, Правило '%s', Пользователь %d: Триггерное сообщение ID %d слишком старое (%v), пропуск.", chatID, ruleName, userID, triggerMessageID, triggerAge)
				}
				ms.addToDeletedCache(chatID, triggerMessageID)
			}
		} else if ms.bot.config.Debug {
			log.Printf("[Moderation Purge DEBUG] Чат %d, Правило '%s', Пользователь %d: Триггерное сообщение ID %d уже в кэше удаленных.", chatID, ruleName, userID, triggerMessageID)
		}
	} else if ms.bot.config.Debug {
		log.Printf("[Moderation Purge DEBUG] Чат %d, Правило '%s', Пользователь %d: Триггерное сообщение ID %d уже было в списке на удаление.", chatID, ruleName, userID, triggerMessageID)
	}

	endTime := time.Now()
	log.Printf("[Moderation Purge] Чат %d, Правило '%s', Пользователь %d (триггер @ %s): Завершена очистка сообщений. Удалено: %d (триггер удален отдельно: %v). Время: %s. Последняя ошибка: %v.",
		chatID, ruleName, userID, triggerTime.Format("15:04:05"), deletedCount, deletedTrigger, endTime.Sub(triggerTime).Round(time.Millisecond), lastErr)
}

// StopPurge останавливает активную задачу очистки для пользователя в чате.
// Возвращает true, если задача была найдена и остановлена, иначе false.
func (ms *ModerationService) StopPurge(chatID int64, userID int64) bool {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if chatPurges, ok := ms.activePurges[chatID]; ok {
		if cancelFunc, ok := chatPurges[userID]; ok {
			log.Printf("[Moderation StopPurge] Чат %d, Пользователь %d: Отмена активной задачи purge.", chatID, userID)
			cancelFunc()               // Вызываем функцию отмены контекста
			delete(chatPurges, userID) // Удаляем запись
			if len(chatPurges) == 0 {
				delete(ms.activePurges, chatID)
			}
			return true // Успешно отменено
		}
	}
	log.Printf("[Moderation StopPurge] Чат %d, Пользователь %d: Активная задача purge не найдена.", chatID, userID)
	return false // Задача не найдена
}

// Дальнейшие методы будут добавлены здесь...
