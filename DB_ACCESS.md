# Подключение к PostgreSQL в Docker на Windows 11

## Вариант 1: Через Docker exec (рекомендуется)

```powershell
# Найти имя контейнера с PostgreSQL
docker ps

# Подключиться к psql внутри контейнера
docker exec -it <container_name> psql -U gwuser -d transactions

# Пример команд в psql:
# Посмотреть таблицы
\dt

# Очистить таблицу транзакций
TRUNCATE TABLE chirp_transactions;

# Посмотреть количество записей
SELECT COUNT(*) FROM chirp_transactions;

# Выход
\q
```

## Вариант 2: Установить PostgreSQL клиент на Windows

### Через Chocolatey:
```powershell
# Установить Chocolatey (если еще не установлен)
Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

# Установить PostgreSQL клиент
choco install postgresql --params '/Password:yourpassword'
```

### Через Scoop:
```powershell
# Установить Scoop (если еще не установлен)
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
irm get.scoop.sh | iex

# Установить PostgreSQL
scoop install postgresql
```

После установки:
```powershell
psql -h localhost -p 5432 -U gwuser -d transactions
# Пароль: gwpass
```

## Вариант 3: Использовать DBeaver или pgAdmin

### DBeaver (рекомендуется):
1. Скачать: https://dbeaver.io/download/
2. Создать новое подключение:
   - Host: localhost
   - Port: 5432
   - Database: transactions
   - Username: gwuser
   - Password: gwpass

### pgAdmin:
1. Скачать: https://www.pgadmin.org/download/pgadmin-4-windows/
2. Добавить новый сервер с теми же параметрами

## Полезные SQL команды

```sql
-- Посмотреть все транзакции
SELECT * FROM chirp_transactions ORDER BY timestamp DESC LIMIT 10;

-- Статистика по типам транзакций
SELECT transaction_type, COUNT(*) as count, SUM(CAST(amount AS DECIMAL)) as total_amount
FROM chirp_transactions
WHERE success = true
GROUP BY transaction_type;

-- Очистить таблицу
TRUNCATE TABLE chirp_transactions;

-- Посмотреть последний checkpoint
SELECT MAX(checkpoint) FROM chirp_transactions;

-- Количество транзакций за последний день
SELECT COUNT(*) FROM chirp_transactions 
WHERE timestamp >= NOW() - INTERVAL '1 day';
```
