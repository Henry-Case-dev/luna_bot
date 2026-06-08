package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
)

func main() {
	fmt.Println("📊 Тестирование создания профиля и обновления LastSeen")

	// Загрузка переменных окружения
	if err := godotenv.Load(); err != nil {
		log.Printf("Предупреждение: Не удалось загрузить .env файл: %v", err)
	}

	// Загрузка конфигурации
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	// Подключение к MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDbURI))
	if err != nil {
		log.Fatalf("❌ Ошибка подключения к MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("❌ MongoDB недоступна: %v", err)
	}

	fmt.Println("✅ Успешно подключились к MongoDB")

	// Прямое подключение к коллекции профилей
	database := client.Database(cfg.MongoDbName)
	profilesCollection := database.Collection(cfg.MongoDbUserProfilesCollection)

	// Тестируем создание нового профиля
	testCreateProfile(profilesCollection)

	// Тестируем обновление LastSeen
	testUpdateLastSeen(profilesCollection)

	fmt.Println("🏁 Тестирование завершено.")
}

func testCreateProfile(collection *mongo.Collection) {
	fmt.Println("\n🧪 Тестирование создания нового профиля")

	testChatID := int64(-999999)
	testUserID := int64(123456789)
	testUsername := "TestUser_CreateProfile"

	// Удаляем тестовый профиль, если существует
	ctx := context.Background()
	filter := bson.M{"chat_id": testChatID, "user_id": testUserID}
	collection.DeleteOne(ctx, filter)

	// Создаем новый профиль с правильными автоматическими полями
	now := time.Now()
	testProfile := storage.UserProfile{
		ChatID:    testChatID,
		UserID:    testUserID,
		Username:  testUsername,
		Alias:     "TestAlias",
		LastSeen:  now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	fmt.Printf("📝 Создаю профиль: User %d (@%s) в чате %d\n", testUserID, testUsername, testChatID)
	fmt.Printf("⏰ CreatedAt: %s\n", testProfile.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("⏰ LastSeen: %s\n", testProfile.LastSeen.Format("2006-01-02 15:04:05"))

	// Прямая вставка в MongoDB с правильным upsert
	update := bson.M{
		"$set": bson.M{
			"username":             testProfile.Username,
			"alias":                testProfile.Alias,
			"gender":               testProfile.Gender,
			"real_name":            testProfile.RealName,
			"bio":                  testProfile.Bio,
			"last_seen":            testProfile.LastSeen,
			"updated_at":           testProfile.UpdatedAt,
			"auto_bio":             testProfile.AutoBio,
			"last_auto_bio_update": testProfile.LastAutoBioUpdate,
		},
		"$setOnInsert": bson.M{
			"created_at": testProfile.CreatedAt, // Используем CreatedAt из профиля
		},
	}

	opts := options.Update().SetUpsert(true)
	result, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		log.Printf("❌ Ошибка создания профиля: %v", err)
		return
	}

	fmt.Printf("✅ Профиль создан (UpsertedCount: %d)\n", result.UpsertedCount)

	// Проверяем созданный профиль
	var createdProfile storage.UserProfile
	err = collection.FindOne(ctx, filter).Decode(&createdProfile)
	if err != nil {
		log.Printf("❌ Ошибка получения созданного профиля: %v", err)
		return
	}

	fmt.Printf("📊 Созданный профиль:\n")
	fmt.Printf("   CreatedAt: %s\n", createdProfile.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   LastSeen: %s\n", createdProfile.LastSeen.Format("2006-01-02 15:04:05"))
	fmt.Printf("   UpdatedAt: %s\n", createdProfile.UpdatedAt.Format("2006-01-02 15:04:05"))

	// Проверяем, что CreatedAt корректно установлен
	timeDiff := createdProfile.CreatedAt.Sub(now).Abs()
	if timeDiff < time.Second {
		fmt.Printf("✅ CreatedAt корректно установлен при создании\n")
	} else {
		fmt.Printf("❌ CreatedAt установлен неправильно. Ожидалось ~%s, получено %s\n",
			now.Format("2006-01-02 15:04:05"), createdProfile.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	// Удаляем тестовый профиль
	collection.DeleteOne(ctx, filter)
	fmt.Printf("🗑️  Тестовый профиль удален\n")
}

func testUpdateLastSeen(collection *mongo.Collection) {
	fmt.Println("\n🧪 Тестирование UpdateUserLastSeen")

	// Используем существующий профиль
	testChatID := int64(-1002661910336)
	testUserID := int64(5885953495)

	ctx := context.Background()
	filter := bson.M{"chat_id": testChatID, "user_id": testUserID}

	// Получаем исходный профиль
	var originalProfile storage.UserProfile
	err := collection.FindOne(ctx, filter).Decode(&originalProfile)
	if err != nil {
		log.Printf("❌ Ошибка получения исходного профиля: %v", err)
		return
	}

	fmt.Printf("📝 Тестируем на профиле: User %d (@%s) в чате %d\n", testUserID, originalProfile.Username, testChatID)
	fmt.Printf("📋 Исходные значения:\n")
	fmt.Printf("   CreatedAt: %s\n", originalProfile.CreatedAt)
	fmt.Printf("   LastSeen: %s\n", originalProfile.LastSeen)
	fmt.Printf("   UpdatedAt: %s\n", originalProfile.UpdatedAt)

	// Обновляем LastSeen (эмулируем логику UpdateUserLastSeen)
	newLastSeen := time.Now()
	fmt.Printf("🔧 Выполняю UpdateUserLastSeen с новым временем: %s\n", newLastSeen.Format("2006-01-02 15:04:05"))

	update := bson.M{
		"$set": bson.M{
			"last_seen":  newLastSeen,
			"updated_at": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Printf("❌ Ошибка обновления LastSeen: %v", err)
		return
	}

	fmt.Printf("✅ UpdateUserLastSeen выполнен. Затронуто строк: %d, изменено: %d\n", result.MatchedCount, result.ModifiedCount)

	// Проверяем обновленный профиль
	var updatedProfile storage.UserProfile
	err = collection.FindOne(ctx, filter).Decode(&updatedProfile)
	if err != nil {
		log.Printf("❌ Ошибка получения обновленного профиля: %v", err)
		return
	}

	fmt.Printf("📊 Результаты после UpdateUserLastSeen:\n")

	// Проверяем, что CreatedAt НЕ изменился
	if updatedProfile.CreatedAt.Equal(originalProfile.CreatedAt) {
		fmt.Printf("   ✅ CreatedAt не изменился: %s\n", updatedProfile.CreatedAt)
	} else {
		fmt.Printf("   ❌ CreatedAt изменился! Было: %s, стало: %s\n",
			originalProfile.CreatedAt, updatedProfile.CreatedAt)
	}

	// Проверяем, что LastSeen обновился
	timeDiff := updatedProfile.LastSeen.Sub(newLastSeen).Abs()
	if timeDiff < time.Second {
		fmt.Printf("   ✅ LastSeen обновился корректно: %s\n", updatedProfile.LastSeen.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("   ❌ LastSeen обновился неправильно. Ожидалось ~%s, получено %s\n",
			newLastSeen.Format("2006-01-02 15:04:05"), updatedProfile.LastSeen.Format("2006-01-02 15:04:05"))
	}

	// Проверяем, что UpdatedAt обновился
	if updatedProfile.UpdatedAt.After(originalProfile.UpdatedAt) {
		fmt.Printf("   ✅ UpdatedAt обновился: %s\n", updatedProfile.UpdatedAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("   ❌ UpdatedAt НЕ обновился. Было: %s, стало: %s\n",
			originalProfile.UpdatedAt.Format("2006-01-02 15:04:05"),
			updatedProfile.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("🎉 ТЕСТ UpdateUserLastSeen ЗАВЕРШЕН!")
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
