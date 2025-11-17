# CHIRP Token Statistics Service

Сервис для получения статистики по токену CHIRP в сети SUI через gRPC и HTTP интерфейсы.

## Описание

Этот сервис собирает и предоставляет информацию о транзакциях токена CHIRP (`0x1ef4c0b20340b8c6a59438204467ca71e1e7cbe918526f9c2c6c5444517cd5ca::chirp::CHIRP`) в сети SUI.

## Функции

- **Синхронизация транзакций** из блокчейна SUI в локальную базу данных PostgreSQL
  - Пакетная обработка чекпоинтов с использованием WaitGroup для параллельной обработки
  - Настраиваемый размер батча для оптимизации производительности
  - Обработка ошибок с продолжением синхронизации
- **Получение транзакций** по адресу кошелька
- **Статистика токена** за неделю или произвольный период:
  - Общее количество заклейменных токенов (claimed)
  - Общее количество переведенных токенов (transferred)
  - Общее количество застейканных токенов (staked)
  - Количество купленных/проданных токенов
  - Количество уникальных держателей
  - Общее количество транзакций
- **Автоматическая синхронизация** с настраиваемым интервалом
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

Скопируйте примеры конфигурации:

```bash
cp app/config/config.yaml.example app/config/config.yaml
cp .env.example .env
```

**Важно**: Все секреты должны быть в `.env` файле, а не в `config.yaml`!

Отредактируйте `.env` файл:
```bash
# Database Configuration (ОБЯЗАТЕЛЬНО)
POSTGRESQL_USER=your_db_user
POSTGRESQL_PASSWORD=your_secure_password
POSTGRESQL_DBNAME=transactions

# Optional: Override defaults
SYNC_BATCH_SIZE=200
SYNC_MAX_WORKERS=10
```

Важные параметры:
- `POSTGRESQL_*` - **обязательные** переменные для подключения к БД
- `SYNC_BATCH_SIZE` - размер батча для обработки checkpoints (100-500)
- `SYNC_MAX_WORKERS` - количество горутин для параллельной обработки (5-20)
- `SYNC_WORKER_TIMEOUT` - таймаут для обработки батча (15s-60s)

Пример `.env` файла:
```bash
POSTGRESQL_USER=chirp_user
POSTGRESQL_PASSWORD=super_secure_password_123
POSTGRESQL_DBNAME=chirp_transactions
SYNC_BATCH_SIZE=200
SYNC_MAX_WORKERS=10
SYNC_WORKER_TIMEOUT=30s
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
POST /api/v1/sync
```

Пример ответа:
```json
{
  "success": true,
  "message": "Synchronization completed successfully"
}
```

## Установка и запуск (Windows)

```cmd
REM 1. Создать конфигурационные файлы
copy app\config\config.yaml.example app\config\config.yaml
copy .env.example .env

REM 2. Отредактировать .env файл (установить пароли БД)
notepad .env

REM 3. Запустить PostgreSQL
docker-compose up -d

REM 4. Сгенерировать protobuf (из корня проекта)
buf generate

REM 5. Перейти в папку app и установить зависимости
cd app
go mod tidy

REM 6. Запустить сервис
go run cmd\go-sui-test\main.go
```

**Обязательно**: Отредактируйте `.env` файл и установите реальные значения для:
- `POSTGRESQL_USER`
- `POSTGRESQL_PASSWORD` 
- `POSTGRESQL_DBNAME`

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

## Производительность и оптимизация

### Пакетная обработка с WaitGroup

Сервис использует `sync.WaitGroup` для параллельной обработки транзакций:

- **Батчи чекпоинтов**: Обрабатываются пачками по `sync.batch_size` чекпоинтов
- **Параллельные воркеры**: До `sync.max_workers` горутин обрабатывают транзакции одновременно
- **Таймауты**: Каждый воркер имеет таймаут `sync.worker_timeout` для предотвращения зависания
- **Graceful degradation**: При ошибке в одном воркере остальные продолжают работу

### Рекомендуемые настройки

Для оптимальной производительности:

```yaml
sync:
  batch_size: 200        # 100-500 в зависимости от нагрузки на SUI RPC
  max_workers: 10        # 5-20 в зависимости от ресурсов сервера
  worker_timeout: "30s"  # 15-60s в зависимости от скорости сети
  interval: "5m"         # Частота автоматической синхронизации
```

### Мониторинг

Логи содержат информацию о:
- Количестве обработанных чекпоинтов
- Времени выполнения батчей
- Ошибках синхронизации
- Статистике найденных CHIRP транзакций

Пример лога:
```
2024/11/17 16:57:00 Processing checkpoint batch 1000-1199 with 10 workers
2024/11/17 16:57:15 Batch 1000-1199 completed: 45 CHIRP transactions found
2024/11/17 16:57:15 Sync completed: processed 200 checkpoints in 15.2s
```

## Безопасность

### Переменные окружения

**Все секреты должны храниться в переменных окружения, а НЕ в config файлах!**

Обязательные переменные:
- `POSTGRESQL_USER` - пользователь БД
- `POSTGRESQL_PASSWORD` - пароль БД (используйте сложный пароль!)
- `POSTGRESQL_DBNAME` - название базы данных

### Файлы, которые НЕ должны попадать в git:

- `.env` - содержит реальные секреты
- `app/config/config.yaml` - может содержать локальные настройки
- `*.log` - логи могут содержать чувствительную информацию

### Рекомендации по безопасности:

1. **Используйте сложные пароли** для БД (минимум 16 символов)
2. **Ограничьте доступ к БД** только с localhost или конкретных IP
3. **Регулярно ротируйте пароли** в production среде
4. **Используйте HTTPS** для HTTP API в production
5. **Настройте TLS** для gRPC в production

### Production deployment:

```bash
# Установите переменные окружения в системе
export POSTGRESQL_PASSWORD="very_secure_password_123!"
export POSTGRESQL_USER="chirp_prod_user"
export POSTGRESQL_DBNAME="chirp_production"

# Запустите сервис
./chirp-transactions-service
```
