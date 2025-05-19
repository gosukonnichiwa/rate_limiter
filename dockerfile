# Используем официальный образ Go для сборки
FROM golang:1.23.4-alpine AS builder

# Устанавливаем зависимости для компиляции (если нужны CGO-библиотеки)
RUN apk add --no-cache gcc musl-dev git

# Создаем рабочую директорию
WORKDIR /app

# Копируем файлы модулей (для кэширования слоя)
COPY go.mod go.sum ./

# Скачиваем зависимости
RUN go mod download

# Копируем весь проект
COPY . .

# Компилируем бинарник с отключенным CGO (для переносимости)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /rate-limiter

# Финальный образ (минимальный alpine)
FROM alpine:3.19

# Устанавливаем tzdata для работы с временными зонами
RUN apk add --no-cache tzdata

# Копируем бинарник и миграции из стадии builder
COPY --from=builder /rate-limiter /app/rate-limiter
COPY --from=builder /app/migrations /app/migrations

# Рабочая директория
WORKDIR /app

# Открываем порт
EXPOSE 8080

# Команда для запуска
CMD ["/app/rate-limiter"]