# TON Tracker Bot

Telegram-бот для отслеживания TON-кошельков с мгновенными уведомлениями о транзакциях.

## Возможности

- **Отслеживание кошельков** — добавляйте TON-адреса и получайте уведомления
- **Уведомления о переводах** — входящие и исходящие TON-транзакции
- **Уведомления о свопах** — обмены на DEX (STON.fi, DeDust, Megaton)
- **Фильтры** — настраиваемый минимальный порог суммы
- **Premium** — расширенные лимиты для активных пользователей
- **Webhooks** — мгновенные уведомления через TonAPI webhooks

## Архитектура

```
ton-tracker/
├── cmd/bot/              # Точка входа
├── internal/
│   ├── config/           # Конфигурация из ENV
│   ├── storage/          # SQLite хранилище
│   ├── tonapi/           # Клиент TonAPI
│   ├── telegram/         # Telegram бот и хендлеры
│   ├── webhook/          # HTTP сервер для webhooks
│   └── notifier/         # Логика уведомлений
└── migrations/           # SQL миграции
```

## Технологии

- **Go 1.22+** — основной язык
- **SQLite** — база данных
- **TonAPI** — блокчейн API
- **go-telegram/bot** — Telegram Bot API

## Быстрый старт

### Требования

- Go 1.22+
- TonAPI ключ (https://tonapi.io)
- Telegram Bot Token ([@BotFather](https://t.me/BotFather))

### Установка

```bash
# Клонировать репозиторий
git clone https://github.com/suspectuso/ton-tracker.git
cd ton-tracker

# Установить зависимости
go mod download

# Скопировать конфиг
cp .env.example .env
```

### Конфигурация

Отредактируйте `.env`:

```env
# Обязательные
BOT_TOKEN=your_telegram_bot_token
TONAPI_API_KEY=your_tonapi_key

# Webhook (для мгновенных уведомлений)
WEBHOOK_ENDPOINT=https://your-domain.com/webhook
WEBHOOK_PORT=8080

# Premium (опционально)
SERVICE_WALLET_ADDR=UQYour_Wallet
PREMIUM_PRICE_TON=5
```

### Запуск

```bash
# Разработка
make run

# Сборка
make build
./ton-tracker

# Docker
docker build -t ton-tracker .
docker run -d --env-file .env -p 8080:8080 ton-tracker
```

## Использование

### Команды бота

- `/start` — главное меню
- `/me` — профиль пользователя

### Inline-кнопки

- ** Добавить кошелёк** — добавить новый адрес
- ** Список кошельков** — управление кошельками
- ** Premium** — информация о Premium

### Настройки кошелька

- ** Минимальная сумма** — фильтр по минимальной сумме транзакции
- ** Сбросить фильтры** — сброс всех настроек

## API

### TonAPI Webhooks

Бот использует TonAPI webhooks для получения событий в реальном времени:

1. При старте создаётся/находится webhook с указанным `WEBHOOK_ENDPOINT`
2. Автоматическая синхронизация подписок с кошельками в БД
3. Входящие события обрабатываются и отправляются пользователям

### Endpoints

- `POST /webhook` — приём событий от TonAPI
- `GET /health` — проверка состояния сервера

## База данных

SQLite с таблицами:

- `wallets` — отслеживаемые кошельки
- `processed_events` — обработанные события (дедупликация)
- `premium_users` — пользователи с Premium
- `premium_payments` — история платежей
- `pending_premium_payments` — ожидающие платежи

## Развертывание

### Systemd

```ini
[Unit]
Description=TON Tracker Bot
After=network.target

[Service]
Type=simple
User=tracker
WorkingDirectory=/opt/ton-tracker
ExecStart=/opt/ton-tracker/ton-tracker
Restart=always
RestartSec=5
EnvironmentFile=/opt/ton-tracker/.env

[Install]
WantedBy=multi-user.target
```

### Docker Compose

```yaml
version: '3.8'
services:
  bot:
    build: .
    restart: always
    env_file: .env
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
```

### Nginx (для webhook)

```nginx
location /webhook {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

## Структура проекта

| Пакет | Описание |
|-------|----------|
| `config` | Загрузка конфигурации из переменных окружения |
| `storage` | Слой работы с SQLite (репозиторий) |
| `tonapi` | HTTP клиент для TonAPI с rate limiting |
| `telegram` | Telegram бот, хендлеры, клавиатуры, FSM |
| `webhook` | HTTP сервер для приёма webhooks, менеджер подписок |
| `notifier` | Парсинг событий, форматирование сообщений |

## Лицензия

MIT

## Автор

[@suspectuso](https://github.com/suspectuso)
