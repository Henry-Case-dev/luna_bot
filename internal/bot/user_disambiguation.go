package bot

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AliasConflict представляет конфликт алиасов в чате
type AliasConflict struct {
	Alias     string
	UserIDs   []int64
	Severity  ConflictSeverity
	Resolved  bool
	CreatedAt time.Time
}

// ConflictSeverity определяет степень серьезности конфликта
type ConflictSeverity int

const (
	ConflictMinor    ConflictSeverity = iota // Только одинаковые FirstName
	ConflictMajor                            // Одинаковые настроенные Alias
	ConflictCritical                         // Активный диалог с одинаковыми именами
)

// UserReferenceValidator система валидации пользовательских ссылок
type UserReferenceValidator struct {
	mu                sync.RWMutex
	chatProfiles      map[int64]map[int64]*storage.UserProfile // chatID -> userID -> profile
	aliasToUserID     map[int64]map[string][]int64             // chatID -> alias -> []userID (конфликты)
	lastUpdate        map[int64]time.Time                      // chatID -> timestamp последнего обновления
	activeConflicts   map[int64][]AliasConflict                // chatID -> []conflicts
	resolutionCache   map[int64]map[string]string              // chatID -> conflictKey -> resolution
	store             storage.ChatHistoryStorage
	cacheValidityTime time.Duration
}

// NewUserReferenceValidator создает новый валидатор
func NewUserReferenceValidator(store storage.ChatHistoryStorage) *UserReferenceValidator {
	return &UserReferenceValidator{
		chatProfiles:      make(map[int64]map[int64]*storage.UserProfile),
		aliasToUserID:     make(map[int64]map[string][]int64),
		lastUpdate:        make(map[int64]time.Time),
		activeConflicts:   make(map[int64][]AliasConflict),
		resolutionCache:   make(map[int64]map[string]string),
		store:             store,
		cacheValidityTime: 5 * time.Minute, // Кеш валиден 5 минут
	}
}

// UpdateChatProfiles обновляет кеш профилей для чата
func (urv *UserReferenceValidator) UpdateChatProfiles(chatID int64) error {
	urv.mu.Lock()
	defer urv.mu.Unlock()

	profiles, err := urv.store.GetAllUserProfiles(chatID)
	if err != nil {
		return fmt.Errorf("ошибка загрузки профилей для чата %d: %w", chatID, err)
	}

	// Инициализируем структуры для чата если нужно
	if urv.chatProfiles[chatID] == nil {
		urv.chatProfiles[chatID] = make(map[int64]*storage.UserProfile)
	}
	if urv.aliasToUserID[chatID] == nil {
		urv.aliasToUserID[chatID] = make(map[string][]int64)
	}
	if urv.resolutionCache[chatID] == nil {
		urv.resolutionCache[chatID] = make(map[string]string)
	}

	// Очищаем старые данные
	urv.chatProfiles[chatID] = make(map[int64]*storage.UserProfile)
	urv.aliasToUserID[chatID] = make(map[string][]int64)

	// Загружаем новые профили
	for _, profile := range profiles {
		urv.chatProfiles[chatID][profile.UserID] = profile

		// Индексируем по всем возможным алиасам
		aliases := urv.getAllPossibleAliases(profile)
		for _, displayName := range aliases {
			if displayName != "" {
				urv.aliasToUserID[chatID][displayName] = append(urv.aliasToUserID[chatID][displayName], profile.UserID)
			}
		}
	}

	urv.lastUpdate[chatID] = time.Now()

	// Обновляем конфликты
	urv.detectAndUpdateConflicts(chatID)

	log.Printf("[DEBUG] UserReferenceValidator: Обновлены профили для чата %d, найдено %d профилей",
		chatID, len(profiles))

	return nil
}

// getAllPossibleAliases возвращает все возможные алиасы пользователя
func (urv *UserReferenceValidator) getAllPossibleAliases(profile *storage.UserProfile) []string {
	var aliases []string

	// Пользовательский алиас (наивысший приоритет)
	if profile.Alias != "" {
		aliases = append(aliases, profile.Alias)
	}

	// RealName из профиля (если есть)
	if profile.RealName != "" {
		aliases = append(aliases, profile.RealName)
	}

	// Username (без @)
	if profile.Username != "" {
		aliases = append(aliases, profile.Username)
	}

	return aliases
}

// CheckAliasConflicts проверяет конфликты алиасов в чате
func (urv *UserReferenceValidator) CheckAliasConflicts(chatID int64) []AliasConflict {
	urv.mu.RLock()
	defer urv.mu.RUnlock()

	if conflicts, exists := urv.activeConflicts[chatID]; exists {
		return conflicts
	}

	return []AliasConflict{}
}

// detectAndUpdateConflicts обнаруживает и обновляет конфликты (вызывается под блокировкой)
func (urv *UserReferenceValidator) detectAndUpdateConflicts(chatID int64) {
	var conflicts []AliasConflict

	aliasMap := urv.aliasToUserID[chatID]
	for alias, userIDs := range aliasMap {
		if len(userIDs) > 1 {
			// Определяем серьезность конфликта
			severity := urv.calculateConflictSeverity(chatID, alias, userIDs)

			conflict := AliasConflict{
				Alias:     alias,
				UserIDs:   userIDs,
				Severity:  severity,
				Resolved:  false,
				CreatedAt: time.Now(),
			}
			conflicts = append(conflicts, conflict)

			log.Printf("[WARN] UserReferenceValidator: Конфликт алиаса '%s' в чате %d между пользователями %v (серьезность: %v)",
				alias, chatID, userIDs, severity)
		}
	}

	urv.activeConflicts[chatID] = conflicts
}

// calculateConflictSeverity определяет серьезность конфликта
func (urv *UserReferenceValidator) calculateConflictSeverity(chatID int64, displayName string, userIDs []int64) ConflictSeverity {
	profilesMap := urv.chatProfiles[chatID]

	// Проверяем, есть ли пользовательские алиасы среди конфликтующих
	hasCustomAlias := false
	for _, userID := range userIDs {
		if profile, exists := profilesMap[userID]; exists && profile.Alias == displayName {
			hasCustomAlias = true
			break
		}
	}

	if hasCustomAlias {
		return ConflictCritical // Пользовательские алиасы - критический конфликт
	}

	// Проверяем количество конфликтующих пользователей
	if len(userIDs) > 2 {
		return ConflictMajor // Много пользователей с одинаковым именем
	}

	return ConflictMinor // Обычный конфликт FirstName
}

// FormatUserWithDisambiguation форматирует пользователя с учетом дисамбигуации для данного контекста
func (urv *UserReferenceValidator) FormatUserWithDisambiguation(chatID int64, userID int64, contextType string, msg *tgbotapi.Message) string {
	urv.ensureCacheValid(chatID)

	urv.mu.RLock()
	defer urv.mu.RUnlock()

	profile := urv.chatProfiles[chatID][userID]
	if profile == nil {
		log.Printf("[WARN][UserDisambiguation] Chat %d: Профиль для пользователя %d не найден", chatID, userID)
		// УЛУЧШЕННЫЙ Fallback при отсутствии профиля
		fallbackAlias := "Неизвестный"
		if msg != nil && msg.From != nil {
			if msg.From.FirstName != "" {
				fallbackAlias = msg.From.FirstName
			} else if msg.From.UserName != "" {
				fallbackAlias = msg.From.UserName
			} else {
				fallbackAlias = fmt.Sprintf("User_%d", userID)
			}
		}

		// Для критичных контекстов всегда добавляем ID
		if contextType == "direct_reply" || contextType == "decision_making" {
			return fmt.Sprintf("%s@U%d", fallbackAlias, userID)
		}
		return fallbackAlias
	}

	// Проверяем конфликты алиасов
	conflicts := urv.CheckAliasConflicts(chatID)
	conflictLevel := ConflictMinor // По умолчанию минимальный уровень конфликта
	hasConflict := false
	conflictingUsers := []int64{}

	for _, conflict := range conflicts {
		for _, conflictUserID := range conflict.UserIDs {
			if conflictUserID == userID {
				conflictLevel = conflict.Severity
				hasConflict = true
				conflictingUsers = conflict.UserIDs
				log.Printf("[DISAMBIGUATION] Chat %d: Пользователь %d (%s) имеет конфликт алиаса уровня %d с %d другими пользователями",
					chatID, userID, profile.Alias, int(conflictLevel), len(conflict.UserIDs)-1)
				break
			}
		}
		if hasConflict {
			break
		}
	}

	// Реализуем правильный fallback: alias -> real_name -> username -> User_ID
	displayName := profile.Alias
	nameSource := "alias"

	if displayName == "" {
		displayName = profile.RealName
		nameSource = "real_name"
	}
	if displayName == "" {
		displayName = profile.Username
		nameSource = "username"
	}
	if displayName == "" {
		displayName = fmt.Sprintf("User_%d", userID)
		nameSource = "fallback"
		log.Printf("[WARN][UserDisambiguation] Chat %d: Все имена пустые для пользователя %d, используем fallback", chatID, userID)
	} else {
		log.Printf("[DEBUG][UserDisambiguation] Chat %d: Используем %s '%s' для пользователя %d", chatID, nameSource, displayName, userID)
	}

	// УЛУЧШЕННАЯ логика форматирования в зависимости от контекста и уровня конфликта
	switch contextType {
	case "decision_making", "free_will":
		// Free Will использует технические теги с именем внутри для четкой привязки
		if msg != nil {
			result := fmt.Sprintf("[MSG:%d][U%d:%s]", msg.MessageID, userID, displayName)
			log.Printf("[DEBUG][UserDisambiguation] Chat %d: Free Will контекст - форматируем как %s для %s@U%d (источник: %s)",
				chatID, result, displayName, userID, nameSource)
			return result
		}
		result := fmt.Sprintf("[U%d:%s]", userID, displayName)
		log.Printf("[DEBUG][UserDisambiguation] Chat %d: Free Will контекст без сообщения - форматируем как %s для %s@U%d (источник: %s)",
			chatID, result, displayName, userID, nameSource)
		return result

	case "direct_reply":
		// КРИТИЧНО: Для прямых ответов всегда используем дисамбигуацию для предотвращения путаницы
		if hasConflict {
			if conflictLevel == ConflictCritical {
				result := fmt.Sprintf("%s@U%d [КОНФЛИКТ:%d]", displayName, userID, len(conflictingUsers))
				log.Printf("[ERROR][UserDisambiguation] Chat %d: Критический конфликт алиаса в direct_reply - форматируем как %s", chatID, result)
				return result
			} else if conflictLevel == ConflictMajor {
				result := fmt.Sprintf("%s@U%d [КОНФЛИКТ]", displayName, userID)
				log.Printf("[WARN][UserDisambiguation] Chat %d: Значительный конфликт алиаса в direct_reply - форматируем как %s", chatID, result)
				return result
			}
		}
		// Даже без конфликтов для direct_reply добавляем ID для безопасности
		result := fmt.Sprintf("%s@U%d", displayName, userID)
		log.Printf("[DEBUG][UserDisambiguation] Chat %d: Direct reply - форматируем как %s для безопасности", chatID, result)
		return result

	case "general", "voice":
		if hasConflict {
			if conflictLevel == ConflictCritical {
				result := fmt.Sprintf("%s@U%d (%d пользователей)", displayName, userID, len(conflictingUsers))
				log.Printf("[WARN][UserDisambiguation] Chat %d: Критический конфликт алиаса в %s - форматируем как %s", chatID, contextType, result)
				return result
			} else if conflictLevel == ConflictMajor {
				result := fmt.Sprintf("%s@U%d", displayName, userID)
				log.Printf("[INFO][UserDisambiguation] Chat %d: Значительный конфликт алиаса в %s - форматируем как %s", chatID, contextType, result)
				return result
			} else {
				result := fmt.Sprintf("%s@U%d", displayName, userID)
				log.Printf("[DEBUG][UserDisambiguation] Chat %d: Незначительный конфликт алиаса в %s - форматируем как %s", chatID, contextType, result)
				return result
			}
		} else {
			// Нет конфликтов - используем только алиас
			log.Printf("[DEBUG][UserDisambiguation] Chat %d: Нет конфликтов для %s@U%d в %s, используем простой алиас", chatID, displayName, userID, contextType)
			return displayName
		}

	default:
		// Fallback для неизвестных контекстов - используем дисамбигуацию для безопасности
		result := fmt.Sprintf("%s@U%d", displayName, userID)
		log.Printf("[WARN][UserDisambiguation] Chat %d: Неизвестный контекст '%s', используем безопасный формат %s",
			chatID, contextType, result)
		return result
	}
}

// FormatReplyReference форматирует ссылку на сообщение в reply chain
func (urv *UserReferenceValidator) FormatReplyReference(chatID int64, replyMsg *tgbotapi.Message) string {
	if replyMsg == nil || replyMsg.From == nil {
		return "Неизвестный"
	}

	urv.ensureCacheValid(chatID)

	userID := replyMsg.From.ID
	baseAlias := urv.FormatUserWithDisambiguation(chatID, userID, "reply_reference", replyMsg)

	// Для reply всегда включаем внутренний маркер для точности
	return fmt.Sprintf("MSG:%d@U%d(%s)", replyMsg.MessageID, userID, strings.Split(baseAlias, "@")[0])
}

// ensureCacheValid проверяет валидность кеша и обновляет при необходимости
func (urv *UserReferenceValidator) ensureCacheValid(chatID int64) {
	urv.mu.RLock()
	lastUpdate, exists := urv.lastUpdate[chatID]
	urv.mu.RUnlock()

	if !exists || time.Since(lastUpdate) > urv.cacheValidityTime {
		// Кеш устарел, обновляем
		if err := urv.UpdateChatProfiles(chatID); err != nil {
			log.Printf("[ERROR] UserReferenceValidator: Ошибка обновления кеша для чата %d: %v", chatID, err)
		}
	}
}

// GetConflictResolution возвращает разрешение конфликта или создает новое
func (urv *UserReferenceValidator) GetConflictResolution(chatID int64, displayName string, userIDs []int64) string {
	urv.mu.RLock()
	if resolutions, exists := urv.resolutionCache[chatID]; exists {
		conflictKey := fmt.Sprintf("%s:%v", displayName, userIDs)
		if resolution, found := resolutions[conflictKey]; found {
			urv.mu.RUnlock()
			return resolution
		}
	}
	urv.mu.RUnlock()

	// Создаем новое разрешение
	resolution := urv.createConflictResolution(chatID, displayName, userIDs)

	urv.mu.Lock()
	if urv.resolutionCache[chatID] == nil {
		urv.resolutionCache[chatID] = make(map[string]string)
	}
	conflictKey := fmt.Sprintf("%s:%v", displayName, userIDs)
	urv.resolutionCache[chatID][conflictKey] = resolution
	urv.mu.Unlock()

	return resolution
}

// createConflictResolution создает разрешение конфликта алиасов
func (urv *UserReferenceValidator) createConflictResolution(chatID int64, displayName string, userIDs []int64) string {
	urv.mu.RLock()
	profilesMap := urv.chatProfiles[chatID]
	urv.mu.RUnlock()

	var resolutionParts []string

	for i, userID := range userIDs {
		var identifier string
		if profile, exists := profilesMap[userID]; exists {
			// Пробуем найти уникальную характеристику
			if profile.Username != "" && profile.Username != displayName {
				identifier = fmt.Sprintf("%s(@%s)", displayName, profile.Username)
			} else if profile.Alias != "" && profile.Alias != displayName {
				identifier = fmt.Sprintf("%s(как %s)", displayName, profile.Alias)
			} else {
				identifier = fmt.Sprintf("%s#%d", displayName, i+1)
			}
		} else {
			identifier = fmt.Sprintf("%s#%d", displayName, i+1)
		}
		resolutionParts = append(resolutionParts, fmt.Sprintf("%s@U%d", identifier, userID))
	}

	return strings.Join(resolutionParts, " vs ")
}

// LogConflictWarning логирует предупреждение о конфликте для LLM
func (urv *UserReferenceValidator) LogConflictWarning(chatID int64) string {
	conflicts := urv.CheckAliasConflicts(chatID)
	if len(conflicts) == 0 {
		return ""
	}

	var warnings []string
	for _, conflict := range conflicts {
		if conflict.Severity >= ConflictMajor {
			resolution := urv.GetConflictResolution(chatID, conflict.Alias, conflict.UserIDs)
			warning := fmt.Sprintf("⚠️ КОНФЛИКТ ИМЕН: '%s' используют %d человека (%s)",
				conflict.Alias, len(conflict.UserIDs), resolution)
			warnings = append(warnings, warning)
		}
	}

	if len(warnings) > 0 {
		return "=== ПРЕДУПРЕЖДЕНИЯ О КОНФЛИКТАХ ИМЕН ===\n" +
			strings.Join(warnings, "\n") +
			"\n=== КОНЕЦ ПРЕДУПРЕЖДЕНИЙ ===\n\n"
	}

	return ""
}
