# CHIRP Token Statistics Service

Сервис для получения статистики по токену CHIRP в сети SUI через gRPC и HTTP интерфейсы.

## Описание

Этот сервис собирает и предоставляет информацию о транзакциях токена CHIRP (`0x1ef4c0b20340b8c6a59438204467ca71e1e7cbe918526f9c2c6c5444517cd5ca::chirp::CHIRP`) в сети SUI.

## Функции

- **Синхронизация транзакций** из блокчейна SUI в локальную базу данных PostgreSQL
- **Получение транзакций** по адресу кошелька
- **Статистика токена** за неделю или произвольный период:
  - Общее количество заклейменных токенов (claimed)
  - Общее количество переведенных токенов (transferred)
  - Общее количество застейканных токенов (staked)
  - Количество купленных/проданных токенов
  - Количество уникальных держателей
  - Общее количество транзакций
- **Автоматическая синхронизация** каждые 5 минут
- **gRPC и HTTP API** для доступа к данным

## Архитектура

Проект следует принципам чистой архитектуры:

```
app/
├── api/grpc/              # Protobuf определения
├── cmd/go-sui-test/       # Точка входа приложения
├── config/                # Конфигурационные файлы
├── internal/
│   ├── config/           # Загрузка конфигурации
│   ├── domain/           # Доменные модели
│   ├── handler/          # HTTP и gRPC обработчики
│   │   ├── grpc/
│   │   └── http/
│   ├── infrastructure/   # Внешние зависимости
│   │   ├── blockchain/   # SUI клиент
│   │   └── storage/      # База данных
│   └── service/          # Бизнес-логика
└── pkg/                  # Сгенерированный код protobuf
```

## Установка и запуск

### Требования

- Go 1.23+
- PostgreSQL 16+
- Docker (опционально)

### 1. Запуск базы данных

```bash
docker-compose up -d
```

### 2. Настройка конфигурации

Скопируйте пример конфигурации:

```bash
cp app/config/config.yaml.example app/config/config.yaml
```

### 3. Генерация protobuf кода

```bash
cd app
buf generate
```

### 4. Установка зависимостей

```bash
cd app
go mod download
```

### 5. Запуск сервиса

```bash
cd app
go run cmd/go-sui-test/main.go
```

Сервис запустится на:
- gRPC: `localhost:50051`
- HTTP: `localhost:8080`

## API

### HTTP Endpoints

#### Получить транзакции по адресу

```bash
GET /api/v1/transactions/address?address=0x...&limit=50&offset=0
```

#### Получить статистику за неделю

```bash
GET /api/v1/statistics/weekly
```

#### Получить статистику за период

```bash
GET /api/v1/statistics?start=2024-01-01T00:00:00Z&end=2024-01-31T23:59:59Z
```

#### Запустить синхронизацию

```bash
REM 1. Создать конфиг (если ещё не создан)
copy app\config\config.yaml.example app\config\config.yaml

REM 2. Запустить PostgreSQL
docker-compose up -d

REM 3. Перейти в папку app
cd app

REM 4. Сгенерировать protobuf (из папки app, но указываем путь к корню)
cd ..
buf generate
cd app

REM 5. Установить зависимости
go mod download
go mod tidy

REM 6. Запустить сервис
go run cmd\go-sui-test\main.go
```

#### Health check

```bash
GET /health
```

### gRPC API

Protobuf определения находятся в `app/api/grpc/transactions.proto`.

Доступные методы:
- `GetTransactionsByAddress` - получить транзакции по адресу
- `GetWeeklyStatistics` - статистика за неделю
- `GetStatistics` - статистика за период
- `SyncTransactions` - запустить синхронизацию

## Типы транзакций

- `claim` - получение токенов через функцию claim
- `transfer` - перевод токенов
- `stake` - стейкинг токенов
- `unstake` - анстейкинг токенов
- `buy` - покупка токенов
- `sell` - продажа токенов
