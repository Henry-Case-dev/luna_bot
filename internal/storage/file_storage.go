package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StoredMessage представляет сообщение для хранения в файле
type StoredMessage struct {
	ChatID           int64  `json:"chat_id"`
	MessageID        int    `json:"message_id"`
	UserID           int64  `json:"user_id,omitempty"`
	Username         string `json:"username,omitempty"`
	FirstName        string `json:"first_name,omitempty"`
	LastName         string `json:"last_name,omitempty"`
	IsBot            bool   `json:"is_bot,omitempty"`
	Text             string `json:"text,omitempty"`
	Date             int    `json:"date"`
	ReplyToMessageID int    `json:"reply_to_message_id,omitempty"`
	RawMessageJSON   string `json:"raw_message_json,omitempty"` // Для хранения всего сообщения

	// Поля для пересылки
	IsForward              bool  `json:"is_forward,omitempty"`
	ForwardedFromUserID    int64 `json:"forwarded_from_user_id,omitempty"`
	ForwardedFromChatID    int64 `json:"forwarded_from_chat_id,omitempty"`
	ForwardedFromMessageID int   `json:"forwarded_from_message_id,omitempty"`
	ForwardedDate          int   `json:"forwarded_date,omitempty"`
}

// ConvertToStoredMessage преобразует tgbotapi.Message в StoredMessage
func ConvertToStoredMessage(msg *tgbotapi.Message) *StoredMessage {
	if msg == nil {
		return nil
	}

	storedMsg := &StoredMessage{
		ChatID:    msg.Chat.ID,
		MessageID: msg.MessageID,
		Text:      msg.Text, // Сохраняем и Text, и Caption
		Date:      msg.Date,
	}
	if msg.From != nil {
		storedMsg.UserID = msg.From.ID
		storedMsg.Username = msg.From.UserName
		storedMsg.FirstName = msg.From.FirstName
		storedMsg.LastName = msg.From.LastName
		storedMsg.IsBot = msg.From.IsBot
	}
	if msg.ReplyToMessage != nil {
		storedMsg.ReplyToMessageID = msg.ReplyToMessage.MessageID
	}

	// Сохраняем информацию о пересылке
	if msg.ForwardDate > 0 {
		storedMsg.IsForward = true
		storedMsg.ForwardedDate = msg.ForwardDate
		if msg.ForwardFrom != nil {
			storedMsg.ForwardedFromUserID = msg.ForwardFrom.ID
		}
		if msg.ForwardFromChat != nil {
			storedMsg.ForwardedFromChatID = msg.ForwardFromChat.ID
		}
		storedMsg.ForwardedFromMessageID = msg.ForwardFromMessageID
	}

	// Сериализуем всё сообщение в JSON для RawMessageJSON
	rawJSONBytes, err := json.Marshal(msg)
	if err == nil {
		storedMsg.RawMessageJSON = string(rawJSONBytes)
	} else {
		log.Printf("Error marshaling raw message for chat %d, msg %d: %v", msg.Chat.ID, msg.MessageID, err)
		// Можно решить, что делать в случае ошибки - пропустить или записать пустую строку
		storedMsg.RawMessageJSON = ""
	}

	return storedMsg
}

// ConvertToAPIMessage преобразует StoredMessage обратно в *tgbotapi.Message.
// Основная логика теперь полагается на десериализацию из RawMessageJSON,
// но мы сохраняем базовую конвертацию для обратной совместимости или случаев,
// когда RawMessageJSON отсутствует.
func ConvertToAPIMessage(stored *StoredMessage) *tgbotapi.Message {
	if stored == nil {
		return nil
	}

	// Пытаемся десериализовать из RawMessageJSON в первую очередь
	if stored.RawMessageJSON != "" {
		var msg tgbotapi.Message
		err := json.Unmarshal([]byte(stored.RawMessageJSON), &msg)
		if err == nil {
			return &msg // Успешно десериализовано
		}
		log.Printf("Error unmarshaling raw message for chat %d, msg %d: %v. Falling back to manual conversion.", stored.ChatID, stored.MessageID, err)
	}

	// Fallback: ручное восстановление из полей StoredMessage
	msg := &tgbotapi.Message{
		MessageID: stored.MessageID,
		From: &tgbotapi.User{
			ID:        stored.UserID,
			IsBot:     stored.IsBot,
			FirstName: stored.FirstName,
			LastName:  stored.LastName,
			UserName:  stored.Username,
		},
		Chat: &tgbotapi.Chat{
			ID: stored.ChatID,
			// Другие поля Chat могут быть недоступны в StoredMessage
		},
		Date: stored.Date,
		Text: stored.Text,
		// Entities и другие поля будут пустыми при ручном восстановлении
	}

	if stored.ReplyToMessageID != 0 {
		// Создаем "пустое" сообщение для ReplyTo, так как полных данных нет
		msg.ReplyToMessage = &tgbotapi.Message{
			MessageID: stored.ReplyToMessageID,
			Chat:      msg.Chat, // Предполагаем, что ответ в том же чате
		}
	}

	// Восстанавливаем информацию о пересылке
	if stored.IsForward {
		msg.ForwardDate = stored.ForwardedDate
		msg.ForwardFromMessageID = stored.ForwardedFromMessageID
		if stored.ForwardedFromUserID != 0 {
			msg.ForwardFrom = &tgbotapi.User{ID: stored.ForwardedFromUserID}
			// Остальные поля ForwardFrom User неизвестны
		}
		if stored.ForwardedFromChatID != 0 {
			msg.ForwardFromChat = &tgbotapi.Chat{ID: stored.ForwardedFromChatID}
			// Остальные поля ForwardFromChat неизвестны
		}
	}

	return msg
}

// FileStorage реализует интерфейс ChatHistoryStorage для хранения данных в файлах JSON.
type FileStorage struct {
	messages      map[int64][]*tgbotapi.Message
	userProfiles  map[int64]map[int64]*UserProfile // map[chatID]map[userID]UserProfile
	contextWindow int
	mutex         sync.RWMutex
	autoSave      bool
	debug         bool
}

// Убедимся, что FileStorage реализует интерфейс ChatHistoryStorage.
var _ ChatHistoryStorage = (*FileStorage)(nil)

// NewFileStorage создает новое файловое хранилище.
func NewFileStorage(contextWindow int, autoSave bool) *FileStorage {
	fs := &FileStorage{
		messages:      make(map[int64][]*tgbotapi.Message),
		userProfiles:  make(map[int64]map[int64]*UserProfile),
		contextWindow: contextWindow,
		autoSave:      autoSave,
	}
	// Загрузка существующих историй при старте
	// TODO: Пересмотреть логику загрузки при старте
	return fs
}

// AddMessage добавляет сообщение в историю чата в памяти.
func (fs *FileStorage) AddMessage(chatID int64, message *tgbotapi.Message) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	fs.messages[chatID] = append(fs.messages[chatID], message)
	// Ограничиваем размер истории
	if len(fs.messages[chatID]) > fs.contextWindow {
		fs.messages[chatID] = fs.messages[chatID][len(fs.messages[chatID])-fs.contextWindow:]
	}
}

// AddVoiceTranscriptionMessage добавляет расшифровку голосового сообщения
// (для FileStorage пока используется обычный AddMessage)
func (fs *FileStorage) AddVoiceTranscriptionMessage(chatID int64, transcriptionMessage *tgbotapi.Message, originalVoiceUserID int64) {
	// Для FileStorage пока просто используем обычный AddMessage
	// В будущем можно расширить формат файла для поддержки флагов расшифровки
	fs.AddMessage(chatID, transcriptionMessage)
}

// GetMessages возвращает последние N сообщений для указанного чата из памяти.
func (fs *FileStorage) GetMessages(chatID int64, limit int) ([]*tgbotapi.Message, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	messages, exists := fs.messages[chatID]
	if !exists {
		return nil, nil // Возвращаем nil, nil, если истории для чата нет
	}

	// Копируем срез, чтобы избежать гонки данных при возврате
	// и обрезаем до лимита, если нужно
	numMessages := len(messages)
	start := 0
	if numMessages > limit {
		start = numMessages - limit
	}
	msgsCopy := make([]*tgbotapi.Message, numMessages-start)
	copy(msgsCopy, messages[start:])

	// Сообщения в file storage хранятся в хронологическом порядке (старые -> новые)
	// Возвращаем последние 'limit' сообщений
	return msgsCopy, nil
}

// GetMessagesSince возвращает сообщения из указанного чата, начиная с определенного времени.
// Добавляем context.Context как первый параметр (хотя он не используется)
// Добавляем userID и limit для соответствия интерфейсу, но они не используются.
func (fs *FileStorage) GetMessagesSince(ctx context.Context, chatID int64, userID int64, since time.Time, limit int) ([]*tgbotapi.Message, error) {
	// Игнорируем userID и limit, так как FileStorage не поддерживает эту фильтрацию.
	if fs.debug {
		log.Printf("[FileStorage GetMessagesSince DEBUG] Called for chat %d, user %d, since %v, limit %d (userID/limit ignored)", chatID, userID, since, limit)
	}
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	messages, exists := fs.messages[chatID]
	if !exists {
		return nil, nil // Нет истории для чата
	}

	// Ищем индекс первого сообщения, которое >= since
	startIndex := -1
	for i, msg := range messages {
		if time.Unix(int64(msg.Date), 0).After(since) || time.Unix(int64(msg.Date), 0).Equal(since) {
			startIndex = i
			break
		}
	}

	if startIndex == -1 {
		return nil, nil // Нет сообщений после указанной даты
	}

	// Копируем срез, чтобы избежать гонки данных при возврате
	msgsCopy := make([]*tgbotapi.Message, len(messages)-startIndex)
	copy(msgsCopy, messages[startIndex:])

	return msgsCopy, nil // Возвращаем (slice, nil)
}

// AddMessagesToContext добавляет предоставленные сообщения в контекст чата.
func (fs *FileStorage) AddMessagesToContext(chatID int64, messages []*tgbotapi.Message) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	fs.messages[chatID] = messages

	// Ограничиваем размер истории после добавления
	if len(fs.messages[chatID]) > fs.contextWindow {
		fs.messages[chatID] = fs.messages[chatID][len(fs.messages[chatID])-fs.contextWindow:]
	}
}

// ClearChatHistory очищает историю сообщений для чата в памяти и удаляет файл.
func (fs *FileStorage) ClearChatHistory(chatID int64) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	delete(fs.messages, chatID)

	// Удаляем файл истории, если он существует
	if err := fs.deleteChatHistoryFile(chatID); err != nil {
		// Логируем ошибку, но не считаем её критичной для очистки памяти
		log.Printf("[FileStorage ClearHistory WARN] Ошибка удаления файла истории для chatID %d: %v", chatID, err)
	}
	return nil
}

// Close в FileStorage ничего не делает, но должен быть для интерфейса.
func (fs *FileStorage) Close() error {
	if fs.autoSave {
		log.Println("FileStorage: Сохранение всех историй перед закрытием...")
		for chatID := range fs.messages {
			if err := fs.SaveChatHistory(chatID); err != nil {
				log.Printf("[FileStorage Close ERROR] Ошибка сохранения истории для chatID %d: %v", chatID, err)
			}
		}
		log.Println("FileStorage: Сохранение завершено.")
	}
	return nil
}

// SaveChatHistory сохраняет историю сообщений чата в JSON файл.
func (fs *FileStorage) SaveChatHistory(chatID int64) error {
	fs.mutex.RLock() // Блокируем на чтение
	messages, exists := fs.messages[chatID]
	fs.mutex.RUnlock()

	if !exists {
		// Если истории нет, ничего не сохраняем (или удаляем файл?) Пока ничего.
		return nil
	}

	// Конвертируем сообщения в формат для хранения
	storedMessages := make([]*StoredMessage, 0, len(messages))
	for _, msg := range messages {
		storedMessages = append(storedMessages, ConvertToStoredMessage(msg))
	}

	data, err := json.MarshalIndent(storedMessages, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка маршалинга истории чата %d: %w", chatID, err)
	}

	filename := fs.getChatHistoryFilename(chatID)
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("ошибка записи истории чата %d в файл %s: %w", chatID, filename, err)
	}

	return nil
}

// LoadChatHistory загружает историю сообщений из JSON файла.
func (fs *FileStorage) LoadChatHistory(chatID int64) ([]*tgbotapi.Message, error) {
	filename := fs.getChatHistoryFilename(chatID)
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Файл не найден - это не ошибка, просто нет истории
		}
		return nil, fmt.Errorf("ошибка чтения истории чата %d из файла %s: %w", chatID, filename, err)
	}

	if len(data) == 0 {
		return []*tgbotapi.Message{}, nil // Пустой файл - пустая история
	}

	var storedMessages []*StoredMessage
	if err := json.Unmarshal(data, &storedMessages); err != nil {
		return nil, fmt.Errorf("ошибка демаршалинга истории чата %d из файла %s: %w", chatID, filename, err)
	}

	// Конвертируем обратно в формат API
	messages := make([]*tgbotapi.Message, 0, len(storedMessages))
	for _, stored := range storedMessages {
		messages = append(messages, ConvertToAPIMessage(stored))
	}

	return messages, nil
}

// getChatHistoryFilename возвращает имя файла для истории чата.
func (fs *FileStorage) getChatHistoryFilename(chatID int64) string {
	// Создаем папку data, если ее нет
	if _, err := os.Stat("data"); os.IsNotExist(err) {
		_ = os.Mkdir("data", 0755)
	}
	return filepath.Join("data", fmt.Sprintf("%d.json", chatID))
}

// deleteChatHistoryFile удаляет файл истории чата.
func (fs *FileStorage) deleteChatHistoryFile(chatID int64) error {
	filename := fs.getChatHistoryFilename(chatID)
	err := os.Remove(filename)
	if err != nil && !os.IsNotExist(err) {
		return err // Возвращаем ошибку, если она не "файл не найден"
	}
	return nil
}

// --- User Profile Methods (FileStorage - In-Memory) ---

// GetUserProfile возвращает профиль пользователя из памяти.
func (fs *FileStorage) GetUserProfile(chatID int64, userID int64) (*UserProfile, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	if profiles, ok := fs.userProfiles[chatID]; ok {
		if profile, ok := profiles[userID]; ok {
			// Возвращаем копию, чтобы избежать изменения вне мьютекса
			profileCopy := *profile
			return &profileCopy, nil
		}
	}
	return nil, nil // Профиль не найден
}

// SetUserProfile сохраняет профиль пользователя в память.
func (fs *FileStorage) SetUserProfile(profile *UserProfile) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if _, ok := fs.userProfiles[profile.ChatID]; !ok {
		fs.userProfiles[profile.ChatID] = make(map[int64]*UserProfile)
	}
	// Сохраняем копию
	profileCopy := *profile
	fs.userProfiles[profile.ChatID][profile.UserID] = &profileCopy
	// TODO: Добавить сохранение профилей в файл?
	return nil
}

// GetAllUserProfiles возвращает все профили пользователей для чата из памяти.
func (fs *FileStorage) GetAllUserProfiles(chatID int64) ([]*UserProfile, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	if profilesMap, ok := fs.userProfiles[chatID]; ok {
		profiles := make([]*UserProfile, 0, len(profilesMap))
		for _, profile := range profilesMap {
			// Возвращаем копии
			profileCopy := *profile
			profiles = append(profiles, &profileCopy)
		}
		return profiles, nil
	}
	return []*UserProfile{}, nil // Возвращаем пустой срез, если для чата нет профилей
}

// GetAllChatIDs возвращает все ID чатов, для которых есть история в памяти.
func (fs *FileStorage) GetAllChatIDs() ([]int64, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	chatIDs := make([]int64, 0, len(fs.messages))
	for chatID := range fs.messages {
		chatIDs = append(chatIDs, chatID)
	}
	return chatIDs, nil
}

// GetStatus возвращает строку со статусом FileStorage.
func (fs *FileStorage) GetStatus(chatID int64) string {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	msgCount := 0
	if msgs, ok := fs.messages[chatID]; ok {
		msgCount = len(msgs)
	}
	profileCount := 0
	if profiles, ok := fs.userProfiles[chatID]; ok {
		profileCount = len(profiles)
	}

	return fmt.Sprintf("FileStorage | Сообщ: %d/%d | Профили: %d | Автосохр: %t",
		msgCount, fs.contextWindow, profileCount, fs.autoSave)
}

// --- Chat Settings Methods (FileStorage - Not Implemented) ---

func (fs *FileStorage) GetChatSettings(chatID int64) (*ChatSettings, error) {
	// FileStorage не хранит настройки чатов персистентно.
	// Возвращаем nil и nil, чтобы вызывающий код использовал дефолтные настройки.
	return nil, nil
}

func (fs *FileStorage) SetChatSettings(settings *ChatSettings) error {
	log.Printf("[FileStorage WARN] SetChatSettings не поддерживается FileStorage для chatID %d", settings.ChatID)
	return fmt.Errorf("SetChatSettings не поддерживается FileStorage")
}

// === Associative Memory Graph (stubs) ===
func (fs *FileStorage) GetAssocTopForContext(chatID int64, contextKeys []string, limit int, freshnessDays int, types []string) ([]*AssocNode, []*AssocEdge, error) {
	log.Printf("[FileStorage WARNING] Associative graph not supported for file storage; returning empty result")
	return []*AssocNode{}, []*AssocEdge{}, nil
}

func (fs *FileStorage) UpdateAssocGraph(chatID int64, updates *AssocUpdateBatch) error {
	// No-op for file storage
	return nil
}

func (fs *FileStorage) UpdateDirectLimitEnabled(chatID int64, enabled bool) error {
	log.Printf("[FileStorage WARN] UpdateDirectLimitEnabled не поддерживается FileStorage для chatID %d", chatID)
	return fmt.Errorf("UpdateDirectLimitEnabled не поддерживается FileStorage")
}

func (fs *FileStorage) UpdateDirectLimitCount(chatID int64, count int) error {
	log.Printf("[FileStorage WARN] UpdateDirectLimitCount не поддерживается FileStorage для chatID %d", chatID)
	return fmt.Errorf("UpdateDirectLimitCount не поддерживается FileStorage")
}

func (fs *FileStorage) UpdateDirectLimitDuration(chatID int64, duration time.Duration) error {
	log.Printf("[FileStorage WARN] UpdateDirectLimitDuration не поддерживается FileStorage для chatID %d", chatID)
	return fmt.Errorf("UpdateDirectLimitDuration не поддерживается FileStorage")
}

func (fs *FileStorage) UpdateVoiceTranscriptionEnabled(chatID int64, enabled bool) error {
	log.Printf("[FileStorage WARN] UpdateVoiceTranscriptionEnabled не поддерживается FileStorage для chatID %d", chatID)
	return fmt.Errorf("UpdateVoiceTranscriptionEnabled не поддерживается FileStorage")
}

func (fs *FileStorage) UpdateSrachAnalysisEnabled(chatID int64, enabled bool) error {
	log.Printf("[FileStorage WARN] UpdateSrachAnalysisEnabled не поддерживается FileStorage для chatID %d", chatID)
	return fmt.Errorf("UpdateSrachAnalysisEnabled не поддерживается FileStorage")
}

func (fs *FileStorage) UpdatePhotoAnalysisEnabled(chatID int64, enabled bool) error {
	log.Printf("[FileStorage WARN] UpdatePhotoAnalysisEnabled не поддерживается FileStorage для chatID %d", chatID)
	return fmt.Errorf("UpdatePhotoAnalysisEnabled не поддерживается FileStorage")
}

// --- Embedding and Vector Search Methods (FileStorage - Stubs) ---

func (fs *FileStorage) SearchRelevantMessages(chatID int64, queryText string, k int) ([]*tgbotapi.Message, error) {
	log.Printf("[FileStorage WARN] SearchRelevantMessages не поддерживается FileStorage для chatID %d", chatID)
	return nil, fmt.Errorf("векторный поиск не поддерживается FileStorage")
}

func (fs *FileStorage) GetTotalMessagesCount(chatID int64) (int64, error) {
	log.Printf("[FileStorage WARN] GetTotalMessagesCount не поддерживается FileStorage для chatID %d", chatID)
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	if msgs, ok := fs.messages[chatID]; ok {
		return int64(len(msgs)), nil
	}
	return 0, nil // Возвращаем 0, если истории нет
}

func (fs *FileStorage) FindMessagesWithoutEmbedding(chatID int64, limit int, skipMessageIDs []int) ([]MongoMessage, error) {
	log.Printf("[FileStorage WARN] FindMessagesWithoutEmbedding не поддерживается FileStorage для chatID %d", chatID)
	return nil, fmt.Errorf("операции с эмбеддингами не поддерживаются FileStorage")
}

// UpdateMessageEmbedding - Заглушка для FileStorage
func (fs *FileStorage) UpdateMessageEmbedding(chatID int64, messageID int, vector []float32) error {
	log.Printf("[FileStorage WARN] UpdateMessageEmbedding не реализован для файлового хранилища.")
	return fmt.Errorf("UpdateMessageEmbedding не реализован для FileStorage")
}

// GetReplyChain - Заглушка для FileStorage.
func (fs *FileStorage) GetReplyChain(ctx context.Context, chatID int64, messageID int, maxDepth int) ([]*tgbotapi.Message, error) {
	log.Printf("[WARN] GetReplyChain не реализован для FileStorage.")
	return nil, errors.New("GetReplyChain не реализован для FileStorage")
}

// ResetAutoBioTimestamps сбрасывает LastAutoBioUpdate для всех пользователей в указанном чате.
func (fs *FileStorage) ResetAutoBioTimestamps(chatID int64) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	count := 0
	if chatProfiles, ok := fs.userProfiles[chatID]; ok {
		for _, profile := range chatProfiles {
			if profile != nil {
				profile.LastAutoBioUpdate = time.Time{}
				count++
			}
		}
	}

	if fs.debug {
		log.Printf("[DEBUG][ResetAutoBio] Chat %d: Успешно сброшено время AutoBio для %d профилей в FileStorage.", chatID, count)
	}

	// В FileStorage нет необходимости в отдельном сохранении, т.к. работаем с памятью.
	// Автосохранение (если включено) сработает при добавлении/закрытии.
	return nil
}

// UpdateAutoBio обновляет только поля auto_bio, last_auto_bio_update и updated_at для указанного пользователя в FileStorage.
func (fs *FileStorage) UpdateAutoBio(ctx context.Context, chatID int64, userID int64, autoBio string, updateTime time.Time) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// Получаем map пользователей для чата
	chatProfiles, ok := fs.userProfiles[chatID]
	if !ok || chatProfiles == nil {
		// Нет профилей для этого чата
		log.Printf("[FileStorage WARN] Чат %d: Профили не найдены при обновлении AutoBio для пользователя %d", chatID, userID)
		return nil // Не ошибка, если профилей нет
	}

	// Получаем профиль пользователя
	profile, ok := chatProfiles[userID]
	if !ok || profile == nil {
		// Профиль пользователя не найден
		log.Printf("[FileStorage WARN] Чат %d: Профиль пользователя %d для обновления AutoBio не найден", chatID, userID)
		return nil // Не ошибка, если профиль не существует
	}

	// Обновляем только нужные поля профиля
	profile.AutoBio = autoBio
	profile.LastAutoBioUpdate = updateTime
	profile.UpdatedAt = updateTime

	// Сохраняем обновленный профиль обратно в мапу (хотя это и не обязательно, т.к. мы работаем с указателем)
	chatProfiles[userID] = profile

	log.Printf("[FileStorage INFO] AutoBio успешно обновлен в памяти: chatID %d, userID %d", chatID, userID)

	if fs.autoSave {
		log.Printf("[FileStorage INFO] Запуск автосохранения после обновления AutoBio: chatID %d, userID %d", chatID, userID)
		// Запускаем сохранение в фоне, чтобы не блокировать вызывающую функцию
		go func() {
			if err := fs.SaveChatHistory(chatID); err != nil {
				log.Printf("[FileStorage ERROR] Ошибка автосохранения после обновления AutoBio: chatID %d, userID %d, error: %v", chatID, userID, err)
			}
		}()
	}

	return nil
}

// UpdateUserLastSeen обновляет только поля last_seen и updated_at для указанного пользователя в FileStorage.
func (fs *FileStorage) UpdateUserLastSeen(chatID int64, userID int64, lastSeen time.Time) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// Получаем map пользователей для чата
	chatProfiles, ok := fs.userProfiles[chatID]
	if !ok || chatProfiles == nil {
		// Нет профилей для этого чата
		log.Printf("[FileStorage WARN] Чат %d: Профили не найдены при обновлении LastSeen для пользователя %d", chatID, userID)
		return nil // Не ошибка, если профилей нет
	}

	// Получаем профиль пользователя
	profile, ok := chatProfiles[userID]
	if !ok || profile == nil {
		// Профиль пользователя не найден
		log.Printf("[FileStorage WARN] Чат %d: Профиль пользователя %d для обновления LastSeen не найден", chatID, userID)
		return nil // Не ошибка, если профиль не существует
	}

	// Обновляем только нужные поля профиля
	profile.LastSeen = lastSeen
	profile.UpdatedAt = time.Now()

	// Сохраняем обновленный профиль обратно в мапу (хотя это и не обязательно, т.к. мы работаем с указателем)
	chatProfiles[userID] = profile

	if fs.debug {
		log.Printf("[FileStorage DEBUG] LastSeen успешно обновлен в памяти: chatID %d, userID %d", chatID, userID)
	}

	if fs.autoSave {
		// Запускаем сохранение в фоне, чтобы не блокировать вызывающую функцию
		go func() {
			if err := fs.SaveChatHistory(chatID); err != nil {
				log.Printf("[FileStorage ERROR] Ошибка автосохранения после обновления LastSeen: chatID %d, userID %d, error: %v", chatID, userID, err)
			}
		}()
	}

	return nil
}

// === НОВЫЕ МЕТОДЫ ДЛЯ PersonalityMemory ===

// GetPersonalityMemory возвращает объект личности бота для конкретного чата
func (fs *FileStorage) GetPersonalityMemory(chatID int64) (*PersonalityMemory, error) {
	// В файловом хранилище создаем память "на лету"
	return &PersonalityMemory{
		ChatID:            chatID,
		NameMentions:      map[string]bool{},
		RecentTopics:      []string{},
		SelfPerception:    []string{},
		DiscussionContext: make(map[string]bool),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}, nil
}

// SavePersonalityMemory сохраняет или обновляет личность бота для чата
func (fs *FileStorage) SavePersonalityMemory(memory *PersonalityMemory) error {
	// В файловом хранилище не сохраняем (временная реализация)
	return nil
}

// UpdatePersonalityField обновляет отдельное поле в личности бота
func (fs *FileStorage) UpdatePersonalityField(chatID int64, fieldName string, value interface{}) error {
	// В файловом хранилище не обновляем (временная реализация)
	return nil
}

// === Методы для работы с реакциями (заглушки) ===
func (fs *FileStorage) UpdateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error {
	return fmt.Errorf("UpdateMessageReactions не реализован для FileStorage")
}

func (fs *FileStorage) GetMessageReactions(chatID int64, messageID int) ([]string, error) {
	return nil, fmt.Errorf("GetMessageReactions не реализован для FileStorage")
}

func (fs *FileStorage) GetBotMessagesWithReactions(chatID int64, lookbackHours int) ([]MongoMessage, error) {
	return nil, fmt.Errorf("GetBotMessagesWithReactions не реализован для FileStorage")
}

// === Методы для работы с казуальной памятью (заглушки) ===
func (fs *FileStorage) GetCausalMemory(chatID int64) (*CausalMemory, error) {
	return nil, fmt.Errorf("CausalMemory не реализована для FileStorage")
}

func (fs *FileStorage) SaveCausalMemory(memory *CausalMemory) error {
	return fmt.Errorf("SaveCausalMemory не реализован для FileStorage")
}

func (fs *FileStorage) AddCausalEntry(entry *CausalMemoryEntry) error {
	return fmt.Errorf("AddCausalEntry не реализован для FileStorage")
}

func (fs *FileStorage) GetCausalEntries(chatID int64, options CausalQueryOptions) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("GetCausalEntries не реализован для FileStorage")
}

func (fs *FileStorage) UpdateCausalEntry(entry *CausalMemoryEntry) error {
	return fmt.Errorf("UpdateCausalEntry не реализован для FileStorage")
}

func (fs *FileStorage) DeleteCausalEntry(chatID int64, entryID int64) error {
	return fmt.Errorf("DeleteCausalEntry не реализован для FileStorage")
}

func (fs *FileStorage) CleanupCausalMemory(chatID int64) error {
	return fmt.Errorf("CleanupCausalMemory не реализован для FileStorage")
}

func (fs *FileStorage) SearchCausalEntries(chatID int64, keywords []string, limit int) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("SearchCausalEntries не реализован для FileStorage")
}

func (fs *FileStorage) GetCausalEntriesByCategory(chatID int64, category string, limit int) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("GetCausalEntriesByCategory не реализован для FileStorage")
}

func (fs *FileStorage) UpdateCausalEntryRelevance(chatID int64, entryID int64, newRelevance float64) error {
	return fmt.Errorf("UpdateCausalEntryRelevance не реализован для FileStorage")
}

func (fs *FileStorage) AddPositiveExample(chatID int64, message string, timestamp time.Time) error {
	return fmt.Errorf("AddPositiveExample не реализован для FileStorage")
}

func (fs *FileStorage) AddNegativeExample(chatID int64, message string, timestamp time.Time) error {
	return fmt.Errorf("AddNegativeExample не реализован для FileStorage")
}

// GetMessageByID получает конкретное сообщение по его ID (заглушка для FileStorage)
func (fs *FileStorage) GetMessageByID(chatID int64, messageID int) (*tgbotapi.Message, error) {
	// Для FileStorage используем GetMessages и ищем нужное сообщение
	messages, err := fs.GetMessages(chatID, 1000) // Получаем больше сообщений для поиска
	if err != nil {
		return nil, err
	}

	for _, msg := range messages {
		if msg.MessageID == messageID {
			return msg, nil
		}
	}

	return nil, nil // Сообщение не найдено
}

// GetMessagesInRange - ЗАГЛУШКА для FileStorage
func (fs *FileStorage) GetMessagesInRange(ctx context.Context, chatID int64, userID int64, since time.Time, until time.Time, limit int) ([]*tgbotapi.Message, error) {
	log.Printf("[FileStorage WARN] GetMessagesInRange не полностью реализован для FileStorage. Будет возвращен пустой список.")
	// TODO: Реализовать фильтрацию по userID, since, until и limit для FileStorage, если потребуется.
	// В текущей реализации FileStorage сложно эффективно фильтровать по диапазону дат и userID без полной загрузки и перебора.
	return []*tgbotapi.Message{}, nil
}

// EnsureTotalDBSizeWithinLimit - ЗАГЛУШКА для FileStorage
func (fs *FileStorage) EnsureTotalDBSizeWithinLimit(cfg *config.Config) (bool, error) {
	log.Printf("[FileStorage WARN] EnsureTotalDBSizeWithinLimit не реализован для FileStorage.")
	// FileStorage не имеет понятия "общего размера БД" в том же смысле, что и MongoDB.
	// Очистка, если потребуется, должна быть реализована на уровне отдельных файлов чатов.
	return false, nil
}

// === Методы для работы с настроением бота (заглушки) ===

// GetMoodState возвращает объект настроения бота для конкретного чата
func (fs *FileStorage) GetMoodState(chatID int64) (*MoodState, error) {
	// В файловом хранилище создаем настроение "на лету"
	return &MoodState{
		ChatID:         chatID,
		CurrentMood:    "neutral",
		MoodIntensity:  0.5,
		LastMoodUpdate: time.Now(),
		TriggerReason:  "Default mood state for FileStorage",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

// SaveMoodState сохраняет или обновляет настроение бота для чата
func (fs *FileStorage) SaveMoodState(mood *MoodState) error {
	// В файловом хранилище не сохраняем (временная реализация)
	if fs.debug {
		log.Printf("[FileStorage DEBUG] SaveMoodState заглушка: chatID %d, mood: %s (%.2f)", mood.ChatID, mood.CurrentMood, mood.MoodIntensity)
	}
	return nil
}

// UpdateMoodState обновляет настроение бота для чата с новыми значениями
func (fs *FileStorage) UpdateMoodState(chatID int64, currentMood string, moodIntensity float64, triggerReason string) error {
	// В файловом хранилище не обновляем (временная реализация)
	if fs.debug {
		log.Printf("[FileStorage DEBUG] UpdateMoodState заглушка: chatID %d, mood: %s (%.2f), reason: %s", chatID, currentMood, moodIntensity, triggerReason)
	}
	return nil
}

// GetDailySummariesForWeek - Stub для FileStorage, возвращает пустой список
func (fs *FileStorage) GetDailySummariesForWeek(ctx context.Context, chatID int64, botUserID int64, since time.Time, until time.Time) ([]*tgbotapi.Message, error) {
	log.Printf("[GetDailySummariesForWeek WARN] Чат %d: FileStorage не поддерживает получение ежедневных саммари", chatID)
	return []*tgbotapi.Message{}, nil
}

// === Методы для работы с эмоциональной системой (заглушки для FileStorage) ===

// GetEmotionalState - заглушка для FileStorage
func (fs *FileStorage) GetEmotionalState(chatID int64) (*EmotionalState, error) {
	log.Printf("[FileStorage WARN] GetEmotionalState не реализован для FileStorage")
	return nil, nil
}

// SaveEmotionalState - заглушка для FileStorage
func (fs *FileStorage) SaveEmotionalState(state *EmotionalState) error {
	log.Printf("[FileStorage WARN] SaveEmotionalState не реализован для FileStorage")
	return nil
}

// UpdateEmotionalState - заглушка для FileStorage
func (fs *FileStorage) UpdateEmotionalState(chatID int64, emotions map[string]float64, intensity float64, triggerEvent string) error {
	log.Printf("[FileStorage WARN] UpdateEmotionalState не реализован для FileStorage")
	return nil
}

// AddEmotionalMemory - заглушка для FileStorage
func (fs *FileStorage) AddEmotionalMemory(memory *EmotionalMemory) error {
	log.Printf("[FileStorage WARN] AddEmotionalMemory не реализован для FileStorage")
	return nil
}

// GetEmotionalMemories - заглушка для FileStorage
func (fs *FileStorage) GetEmotionalMemories(chatID int64, userID int64, limit int) ([]*EmotionalMemory, error) {
	log.Printf("[FileStorage WARN] GetEmotionalMemories не реализован для FileStorage")
	return nil, nil
}

// GetEmotionalMemoriesByEmotion - заглушка для FileStorage
func (fs *FileStorage) GetEmotionalMemoriesByEmotion(chatID int64, emotion string, limit int) ([]*EmotionalMemory, error) {
	log.Printf("[FileStorage WARN] GetEmotionalMemoriesByEmotion не реализован для FileStorage")
	return nil, nil
}

// UpdateEmotionalMemory - заглушка для FileStorage
func (fs *FileStorage) UpdateEmotionalMemory(memory *EmotionalMemory) error {
	log.Printf("[FileStorage WARN] UpdateEmotionalMemory не реализован для FileStorage")
	return nil
}

// CleanupEmotionalMemories - заглушка для FileStorage
func (fs *FileStorage) CleanupEmotionalMemories(chatID int64, maxAge time.Duration) error {
	log.Printf("[FileStorage WARN] CleanupEmotionalMemories не реализован для FileStorage")
	return nil
}

// GetEmotionalTrends - заглушка для FileStorage
func (fs *FileStorage) GetEmotionalTrends(chatID int64, since time.Time, limit int) (map[string]float64, error) {
	log.Printf("[FileStorage WARN] GetEmotionalTrends не реализован для FileStorage")
	return nil, nil
}
