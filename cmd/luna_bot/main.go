package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot"
	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/joho/godotenv"
)

// handleRoot - простой обработчик HTTP запросов
func handleRoot(w http.ResponseWriter, r *http.Request) {
	// Логируем каждый входящий запрос к этому обработчику
	log.Printf("--> HTTP Request Received: Method=%s, Path=%s, RemoteAddr=%s", r.Method, r.URL.Path, r.RemoteAddr)
	fmt.Fprintf(w, "Hello from Luna Bot server!")
	// Логируем после отправки ответа
	log.Printf("<-- HTTP Response Sent for: %s %s", r.Method, r.URL.Path)
}

// handleStatus - обработчик статуса сервисов
func handleStatus(w http.ResponseWriter, r *http.Request) {
	log.Printf("--> Status Request: Method=%s, Path=%s, RemoteAddr=%s", r.Method, r.URL.Path, r.RemoteAddr)

	status := map[string]string{
		"bot":       "running",
		"grafana":   "http://127.0.0.1:3000/",
		"loki":      "http://127.0.0.1:3100/",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": %q, "services": {"grafana": %q, "loki": %q}, "timestamp": %q}`,
		status["bot"], status["grafana"], status["loki"], status["timestamp"])

	log.Printf("<-- Status Response Sent for: %s %s", r.Method, r.URL.Path)
}

func main() {
	// Добавить эти строки в самое начало main()
	_ = godotenv.Load(".env")
	_ = godotenv.Load(".env.secrets")

	// Определяем путь к логам в зависимости от окружения
	logfilePath := "luna_runtime.log"
	if os.Getenv("PRODUCTION") == "true" || os.Getenv("DOCKER") == "true" {
		// В production/Docker логи в стандартной директории
		logfilePath = "/var/log/luna_bot/luna_runtime.log"
		// Создаем директорию если её нет
		if err := os.MkdirAll("/var/log/rofloslav", 0755); err != nil {
			log.Printf("Не удалось создать директорию логов: %v", err)
		}
	}

	// Удаляем старый файл логов, если он существует
	if _, err := os.Stat(logfilePath); err == nil {
		if err := os.Remove(logfilePath); err != nil {
			log.Printf("Не удалось удалить старый файл логов '%s': %v", logfilePath, err)
			// Продолжаем работу, даже если не удалось удалить старый файл
		} else {
			log.Printf("Старый файл логов '%s' успешно удален.", logfilePath)
		}
	}

	f, err := os.OpenFile(logfilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		log.Printf("Не удалось открыть файл логов '%s': %v", logfilePath, err)
		// Если не удалось открыть файл логов, продолжаем работу с выводом в stdout
		log.SetOutput(os.Stdout)
	} else {
		defer f.Close()
		log.SetOutput(f) // Перенаправляем все вызовы log.* в файл
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds) // Используем стандартные флаги + микросекунды для большей точности

	log.Println("=== Application Starting ===")
	log.Printf("Timestamp: %s", time.Now().UTC().Format(time.RFC3339))

	cfg, err := config.Load()
	if err != nil {
		log.Printf("!!! FATAL: Ошибка загрузки конфигурации: %v", err)
		time.Sleep(15 * time.Second)
		panic(fmt.Sprintf("Configuration error: %v", err))
	}
	log.Println("--- Configuration Loaded ---")

	botInstance, err := bot.New(cfg)
	if err != nil {
		log.Printf("!!! FATAL: Ошибка создания бота: %v", err)
		time.Sleep(15 * time.Second)
		panic(fmt.Sprintf("Bot creation error: %v", err))
	}
	log.Println("--- Bot Initialized ---")

	// Запускаем бота в отдельной горутине
	go func() {
		if startErr := botInstance.Start(); startErr != nil {
			log.Printf("!!! CRITICAL: Критическая ошибка запуска бота: %v", startErr)
		}
	}()
	log.Println("--- Bot Start Goroutine Launched ---")
	log.Println("Бот запущен.")

	// --- Запуск Dummy HTTP сервера ---
	http.HandleFunc("/", handleRoot)         // Регистрируем обработчик
	http.HandleFunc("/status", handleStatus) // Статус сервисов

	// Выбор порта в зависимости от режима
	serverAddr := ":8080" // Дефолтный порт для микросервисной архитектуры
	if os.Getenv("NGINX_MODE") == "true" {
		serverAddr = ":8080" // Внутренний порт для nginx проксирования (тот же)
	}

	log.Printf("--- Starting HTTP server on %s ---", serverAddr)

	go func() {
		// Логируем перед запуском сервера
		log.Printf("[HTTP Goroutine] Attempting to start HTTP server on %s", serverAddr)
		if httpErr := http.ListenAndServe(serverAddr, nil); httpErr != nil {
			// Логируем ошибку, если ListenAndServe вернул ее
			log.Printf("!!! [HTTP Goroutine] HTTP Server Error: %v", httpErr)
		}
		// Логируем, если ListenAndServe завершился (даже без ошибки, хотя это маловероятно)
		log.Printf("[HTTP Goroutine] ListenAndServe on %s finished.", serverAddr)
	}()
	// Добавляем лог сразу после запуска горутины
	log.Printf("--- HTTP Server Goroutine Launched on %s ---", serverAddr)
	// --- Конец HTTP сервера ---

	log.Printf("--- Application Ready. Waiting indefinitely. ---")

	// Ожидаем бесконечно, игнорируем сигналы завершения.
	// Это нужно для Amvera, чтобы контейнер оставался RUNNING.
	select {}

	// Этот код больше никогда не будет выполнен в Amvera.
	// Оставляем его закомментированным на случай локальных тестов.
	/*
		log.Println("Остановка бота (из main)...") // Добавляем лог для ясности
		bot.Stop()
		log.Println("Приложение остановлено")
	*/
}
