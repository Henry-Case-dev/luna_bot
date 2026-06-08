# ---- Build Stage ----
# Используем официальный образ Go 1.22 на Alpine для сборки
FROM golang:1.22-alpine AS builder

# Устанавливаем рабочую директорию внутри контейнера
WORKDIR /app

# Копируем файлы модулей Go
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download -x

# Копируем остальной исходный код
COPY . .

# Собираем приложение
# CGO_ENABLED=0 - для статической линковки, важно для Alpine
# -ldflags="-w -s" - уменьшает размер бинарника
# -v - verbose output
RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-w -s" -o /luna_bot ./cmd/luna_bot

# ---- Runtime Stage ----
# Используем конкретную версию Alpine для предсказуемости
FROM alpine:3.19

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем скомпилированный бинарник из стадии сборки
COPY --from=builder /luna_bot ./

# Копируем папку с изображениями для донатов
COPY donate_images ./donate_images

# Копируем папку с базовыми изображениями для генерации
COPY luna_appearance ./luna_appearance

# Создаем директории для логов и временных файлов
RUN mkdir -p /var/log/luna_bot /tmp/voice_messages && \
    chmod -R 755 /var/log/luna_bot /tmp/voice_messages

# Устанавливаем только core-зависимости:
# - tzdata: часовые пояса
# - ffmpeg: обработка голосовых сообщений (включает libopus)
# - ca-certificates: HTTPS-запросы к внешним API
# - wget: healthcheck-проверки
RUN apk add --no-cache tzdata ffmpeg ca-certificates wget

# Очищаем кеш пакетного менеджера
RUN rm -rf /var/cache/apk/*

# Проверяем версию ffmpeg (core-зависимость)
RUN ffmpeg -version | awk 'NR==1{print;exit}'

# Создаем непривилегированного пользователя
RUN addgroup -S appgroup && adduser -S appuser -G appgroup && \
    chown -R appuser:appgroup /app /var/log/luna_bot /tmp/voice_messages

# Основной порт приложения
EXPOSE 8080

# Переключаемся на непривилегированного пользователя
USER appuser

# Запускаем бота напрямую
CMD ["/app/luna_bot"]
