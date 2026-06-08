# 🤖 Luna Bot — Telegram-бот с ИИ на Go

[![Go Version](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://github.com/Henry-Case-dev/luna_bot/actions/workflows/go.yml/badge.svg)](https://github.com/Henry-Case-dev/luna_bot/actions/workflows/go.yml)

**Luna** — продвинутый Telegram-бот с когнитивной архитектурой, использующий LLM (Gemini, DeepSeek, OpenRouter) для генерации ответов в групповых чатах. Бот спроектирован для имитации живого общения — ироничного и адаптивного собеседника.

---

## 🚀 Ключевые возможности

- **Участие в беседе** — анализ истории сообщений и ответы в заданном «характере»
- **Прямые ответы** — реакция на упоминания (@botname) и reply на сообщения бота
- **Free Will (автономное поведение)** — двухэтапное принятие решений: (1) нужно ли отвечать, (2) какой тип ответа выбрать (general, direct_reply, silence_response, context_based, take_response)
- **Тема дня** — генерация ежедневной провокационной темы для обсуждения
- **Саммари чата** — краткое изложение диалога за 24 часа + еженедельное саммари
- **Анализ «срачей»** — детекция конфликтов, анализ сторон и аргументов, вердикт
- **Система личности** — статическая основа + динамическая эволюция (характер адаптируется к чату)
- **Дисамбигуация пользователей** — уникальная идентификация участников в контексте LLM
- **Голосовые сообщения** — генерация речи через ElevenLabs TTS API
- **Веб-поиск** — актуальная информация через Google Custom Search API
- **Генерация изображений** — создание изображений через Gemini Image Generation
- **Когнитивная архитектура** — имитация внутреннего монолога, саморефлексии, эмоциональной системы, каузальное обучение
- **Модерация** — автоматическое управление нарушениями (mute, kick, ban, purge)
- **Реакции** — интеллектуальные эмодзи-реакции на сообщения
- **Предотвращение повторений** — интеллектуальная переработка дублирующихся ответов с сохранением стиля

---

## 🛠 Технологический стек

| Компонент | Технология |
|-----------|------------|
| **Язык** | Go 1.22 |
| **LLM** | Google Gemini API, DeepSeek API, OpenRouter |
| **Telegram API** | `go-telegram-bot-api/v5` |
| **База данных** | PostgreSQL 16 + pgvector (основная), файлы JSON (legacy) |
| **TTS** | ElevenLabs API |
| **Поиск** | Google Custom Search API |
| **Конфигурация** | `.env` (godotenv) + модульные `.txt` промпты |
| **Контейнеризация** | Docker (multi-stage) + docker-compose |
| **CI/CD** | GitHub Actions |
| **Мониторинг** | Loki + Promtail + Grafana (опционально) |
| **Линтинг** | golangci-lint (errcheck, govet, staticcheck, misspell, gosimple, gofmt, goimports) |

---

## ⚡ Быстрый старт

### Предварительные требования

- [Docker](https://docs.docker.com/get-docker/) и [docker-compose](https://docs.docker.com/compose/install/)
- Токен Telegram бота (получить у [@BotFather](https://t.me/BotFather))
- API-ключ LLM-провайдера:
  - [Google Gemini API key](https://aistudio.google.com/apikey) (бесплатный тир доступен)
  - Или [DeepSeek API key](https://platform.deepseek.com/)
  - Или [OpenRouter API key](https://openrouter.ai/)

### Установка

```bash
# 1. Клонирование репозитория
git clone https://github.com/Henry-Case-dev/luna_bot.git
cd luna_bot

# 2. Настройка окружения
cp .env.example .env
# Отредактируйте .env — укажите TELEGRAM_TOKEN, GEMINI_API_KEY, POSTGRESQL_PASSWORD

# 3. Запуск бота с PostgreSQL
docker compose up -d

# 4. (Опционально) Запуск с мониторингом (Loki + Grafana)
docker compose -f docker-compose.monitoring.yml up -d
```

### Проверка работы

```bash
# Просмотр логов бота
docker compose logs -f luna_bot

# Проверка статуса
curl http://localhost:8080/status

# Grafana (если запущен мониторинг)
# http://localhost:3000 (логин: admin, пароль: admin123)
```

---

## 📁 Структура проекта (Standard Go Project Layout)

```
luna_bot/
├── cmd/
│   └── luna_bot/
│       └── main.go                  # Точка входа, HTTP-сервер :8080
├── internal/
│   ├── bot/                         # Telegram-бот и все сервисы (44 файла)
│   │   ├── bot.go                   #   Конструктор Bot (God Object)
│   │   ├── free_will.go             #   Free Will — автономное поведение
│   │   ├── cognitive_architecture.go #  Когнитивная архитектура (Этап 3)
│   │   ├── social_architecture.go   #   Социальная архитектура (Этап 4)
│   │   ├── emotional_analyzer.go    #   Эмоциональный анализ (Этап 2)
│   │   ├── causal_analyzer.go       #   Каузальное обучение (Этап 1)
│   │   ├── message_handler.go       #   Обработчик входящих сообщений
│   │   ├── responder.go             #   Генерация ответов
│   │   ├── reaction_*.go            #   Система реакций
│   │   ├── voice_*.go               #   Голосовые сообщения
│   │   ├── prompts/                 #   Модульные промпты (14 .txt + парсер)
│   │   │   ├── prompts.go           #     LoadPrompt(), парсер секций >>>
│   │   │   ├── main.txt             #     Основные промпты (14 секций)
│   │   │   ├── free_will.txt        #     Free Will промпты (12 секций)
│   │   │   ├── personality.txt      #     Промпты личности
│   │   │   ├── emotional.txt        #     Эмоциональные промпты
│   │   │   ├── cognitive.txt        #     Когнитивные промпты
│   │   │   ├── social.txt           #     Социальные промпты
│   │   │   ├── causal.txt           #     Каузальные промпты
│   │   │   ├── srach.txt            #     Промпты анализа срачей
│   │   │   ├── reactions.txt        #     Промпты реакций
│   │   │   ├── anti_repetition.txt  #     Анти-повторения
│   │   │   ├── auto_bio.txt         #     AutoBio промпты
│   │   │   ├── post_processor.txt   #     Пост-обработка сообщений
│   │   │   ├── web_search.txt       #     Веб-поиск
│   │   │   └── image_gen.txt        #     Генерация изображений
│   │   └── ...
│   ├── config/                      # Конфигурация (4 файла)
│   │   ├── types.go                 #   Структура Config (150+ полей)
│   │   ├── load.go                  #   Загрузка из .env + промпты из .txt
│   │   └── validate.go              #   Валидация
│   ├── storage/                     # Слой данных (11 файлов)
│   │   ├── storage.go               #   Интерфейс Storage + все типы данных
│   │   ├── postgres_storage.go      #   Конструктор PostgreSQL + DDL
│   │   ├── postgres_messages.go     #   CRUD сообщений
│   │   ├── postgres_profiles.go     #   Профили пользователей
│   │   ├── postgres_settings.go     #   Настройки чатов
│   │   ├── postgres_personality.go  #   Память личности
│   │   ├── postgres_embeddings.go   #   Векторные эмбеддинги (pgvector)
│   │   ├── postgres_rag.go          #   RAG-поиск
│   │   ├── postgres_causal_memory.go #  Каузальная память
│   │   ├── mock_storage.go          #   Мок для тестов
│   │   └── file_storage.go          #   Файловое хранилище (legacy)
│   ├── llm/                         # LLM-клиенты
│   │   ├── llm.go                   #   Интерфейс LLMClient
│   │   ├── gemini/                  #   Gemini Client
│   │   ├── deepseek/                #   DeepSeek Client
│   │   ├── openrouter/              #   OpenRouter Client
│   │   └── elevenlabs/              #   ElevenLabs TTS Client
│   └── utils/                       # Утилиты (2 файла)
├── configs/                         # Конфиги мониторинга
│   ├── loki-config.yml
│   └── grafana-datasources.yml
├── monitoring/                      # Мониторинг (Dockerfile + конфиги)
│   ├── loki/
│   ├── promtail/
│   └── grafana/
├── docs/                            # Документация
├── plans/                           # Технические планы и беклог
├── donate_images/                   # Изображения для донатов
├── luna_appearance/                 # Базовые изображения для генерации
├── .github/workflows/go.yml         # CI Pipeline (lint + test + build)
├── .golangci.yml                    # Конфигурация линтера
├── docker-compose.yml               # Бот + PostgreSQL 16 (pgvector)
├── docker-compose.monitoring.yml    # Loki + Promtail + Grafana
├── Dockerfile                       # Multi-stage (golang:1.22-alpine → alpine:3.19, 67 строк)
├── Makefile                         # test, build, vet, clean
├── .env.example                     # Шаблон переменных окружения
├── go.mod / go.sum
└── README.md
```

---

## ⚙️ Конфигурация

Конфигурация загружается в следующем порядке приоритета:
1. **Модульные `.txt` промпты** — [`internal/bot/prompts/*.txt`](internal/bot/prompts/) (наивысший приоритет, переопределяют `.env`)
2. **Переменные окружения** — из `.env` (загружается через [`godotenv`](cmd/luna_bot/main.go:44))
3. **Значения по умолчанию** — заданы в [`internal/config/load.go`](internal/config/load.go)

### Ключевые переменные `.env`

```bash
# === Обязательные ===
TELEGRAM_TOKEN=your_bot_token          # Токен Telegram бота (@BotFather)

# === LLM-провайдер (выберите один) ===
LLM_PROVIDER=gemini                    # gemini | deepseek | openrouter
GEMINI_API_KEY=your_gemini_key         # API-ключ Google Gemini
# DEEPSEEK_API_KEY=your_deepseek_key   # API-ключ DeepSeek
# OPENROUTER_API_KEY=your_or_key       # API-ключ OpenRouter

# === База данных ===
STORAGE_TYPE=postgres                  # postgres (основной) | file (legacy)
POSTGRESQL_HOST=localhost              # Хост PostgreSQL (db — в Docker)
POSTGRESQL_PORT=5432
POSTGRESQL_USER=postgres
POSTGRESQL_PASSWORD=your_password
POSTGRESQL_DBNAME=luna_bot

# === Основные настройки ===
TIME_ZONE=Asia/Yekaterinburg           # Часовой пояс
MIN_MESSAGES=10                        # Мин. сообщений перед ответом
MAX_MESSAGES=50                        # Макс. сообщений перед ответом
CONTEXT_WINDOW=300                     # Размер контекстного окна
DEBUG=false                            # Режим отладки
ADMIN_USERNAMES=your_username          # Администраторы (через запятую)

# === Free Will (автономное поведение) ===
FREE_WILL_ENABLED=false                # Включить автономное поведение
FREE_WILL_MIN_INTERVAL_MINUTES=15
FREE_WILL_MAX_INTERVAL_MINUTES=60
FREE_WILL_MAX_DECISIONS_PER_HOUR=10

# === Дополнительные сервисы ===
VOICE_MESSAGES_ENABLED=true            # Голосовые сообщения
# ELEVENLABS_API_KEY=your_key          # API-ключ ElevenLabs TTS
WEB_SEARCH_ENABLED=true                # Веб-поиск
# GOOGLE_SEARCH_API_KEY=your_key
# GOOGLE_SEARCH_ENGINE_ID=your_id
```

Полный список переменных (150+) см. в [`.env.example`](.env.example) и [`internal/config/types.go`](internal/config/types.go).

### Промпты

Промпты хранятся в модульных `.txt` файлах [`internal/bot/prompts/`](internal/bot/prompts/). Формат поддерживает:
- **Отдельные файлы** — `<name>.txt` (старый формат)
- **Секции** — `>>> <name>` в любом `.txt` файле (новый модульный формат, 14 файлов, 53 секции)

Загрузка: [`prompts.LoadPrompt(name)`](internal/bot/prompts/prompts.go:26) → [`config.loadPromptsFromFiles()`](internal/config/load.go:890).

---

## 👨‍💻 Административные команды

### Личность бота
| Команда | Описание |
|---------|----------|
| `/personality_show` | Показать текущую личность |
| `/personality_update_static [текст]` | Обновить статическую часть |
| `/personality_update_style [текст]` | Обновить инструкции поведения |
| `/personality_reset_dynamic` | Сбросить динамическую часть |
| `/personality_stats` | Статистика личности |
| `/update_personality` | Запустить обновление памяти личности |
| `/update_personality all` | Обновить личность для всех чатов |

### Профили и дисамбигуация
| Команда | Описание |
|---------|----------|
| `/profile_set` | Установить профиль пользователя |
| `/disambiguation_status` | Статус сервиса дисамбигуации |
| `/disambiguation_toggle` | Переключить дисамбигуацию |
| `/user_conflicts` | Показать конфликты алиасов |
| `/user_resolve <alias>` | Разрешить конфликт алиаса |
| `/user_cache_refresh` | Обновить кеш профилей |

### AutoBio
| Команда | Описание |
|---------|----------|
| `/trigger_autobio` | Запустить анализ AutoBio |
| `/reset_autobio` | Сбросить метки времени |

### Анти-повторения
| Команда | Описание |
|---------|----------|
| `/antirepetition_stats` | Статистика системы |
| `/antirepetition_toggle` | Включить/выключить |
| `/antirepetition_stats clear` | Очистить записи |

### Модерация
| Команда | Описание |
|---------|----------|
| `/stop_purge @username` | Остановить очистку сообщений |

### Настройки чата
| Команда | Описание |
|---------|----------|
| `/settings` | Настройки чата |
| `/summary` | Создать саммари |

---

## 🧑‍💻 Разработка

### Локальный запуск (без Docker)

```bash
# Требуется: Go 1.22, PostgreSQL 16 + pgvector
cp .env.example .env
# Настройте .env: TELEGRAM_TOKEN, GEMINI_API_KEY, POSTGRESQL_HOST=localhost

go mod download
go run ./cmd/luna_bot
```

### Makefile

```bash
make test             # Юнит-тесты
make test-cover       # Тесты с покрытием
make test-integration # Интеграционные тесты (требуется БД)
make test-all         # Все тесты
make build            # Сборка
make vet              # go vet
make clean            # Очистка
```

### Линтинг

```bash
golangci-lint run ./...
```

Конфигурация: [`.golangci.yml`](.golangci.yml) — errcheck, govet, staticcheck, unused, misspell, gosimple, gofmt, goimports.

---

## 🐳 Docker

### Dockerfile

Multi-stage сборка ([`Dockerfile`](Dockerfile), 67 строк):
- **Stage 1 (builder)**: `golang:1.22-alpine` — сборка статического бинарника
- **Stage 2 (runtime)**: `alpine:3.19` — tzdata, ffmpeg, ca-certificates

```bash
docker build -t luna_bot .
docker run -d --env-file .env -p 8080:8080 --name luna_bot luna_bot
```

### docker-compose

[`docker-compose.yml`](docker-compose.yml) — бот + PostgreSQL 16 с pgvector:
- **luna_bot** — основное приложение, healthcheck через `/status`
- **db** — PostgreSQL 16 (образ `pgvector/pgvector:pg16`), healthcheck через `pg_isready`
- `depends_on` с `condition: service_healthy`

[`docker-compose.monitoring.yml`](docker-compose.monitoring.yml) — опциональный мониторинг:
- **loki** — хранение логов
- **promtail** — сбор логов из контейнеров
- **grafana** — дашборды (порт 3000, логин admin/admin123)

```bash
# Запуск с мониторингом
docker compose up -d
docker compose -f docker-compose.monitoring.yml up -d
```

---

## 🤝 Contributing

Pull request'ы приветствуются! Для крупных изменений сначала создайте issue для обсуждения.

1. Форкните репозиторий
2. Создайте ветку (`git checkout -b feature/amazing-feature`)
3. Закоммитьте изменения (`git commit -m 'Add amazing feature'`)
4. Запушьте ветку (`git push origin feature/amazing-feature`)
5. Откройте Pull Request

Убедитесь, что код проходит линтинг и тесты:
```bash
golangci-lint run ./...
go test ./... -short -count=1
go build ./cmd/luna_bot
```

---

## 📄 Лицензия

MIT License — см. файл [LICENSE](LICENSE).

---

## 📊 Статус проекта

![Progress](https://img.shields.io/badge/Progress-100%25-brightgreen)

Проект прошёл полный цикл подготовки к открытию исходного кода (v1–v6):
- ✅ Санитарная очистка (удаление мусора, бинарников, дубликатов)
- ✅ Восстановление v2-слоя PostgreSQL (11 файлов, pgvector)
- ✅ Standard Go Project Layout
- ✅ Модульные промпты (14 файлов, 53 секции)
- ✅ Линтинг (golangci-lint, goimports, gofmt) — чисто
- ✅ CI/CD Pipeline (GitHub Actions)
- ✅ Clean Dockerfile (multi-stage, 67 строк)
- ✅ docker-compose с PostgreSQL 16 (pgvector) и healthcheck
- ✅ Документация обновлена (README, STATE_SUMMARY, docs)
- ✅ Финальная верификация и Git-инициализация
- ✅ Git remote: [`github.com/Henry-Case-dev/luna_bot`](https://github.com/Henry-Case-dev/luna_bot)

---

*Luna Bot — ИИ-бот для имитации общения в Telegram.*
