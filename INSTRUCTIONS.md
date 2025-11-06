Это проект сервиса для получения через grpc и http интерфейсы информации о транзакциях токена в сети sui

В сети SUI есть токен, его адрес 0x1ef4c0b20340b8c6a59438204467ca71e1e7cbe918526f9c2c6c5444517cd5ca::chirp::CHIRP

Нужно научиться получать из сети статистику по этому токену. Придумай какую. Конечная цель - определить, сколько продано, сколько куплено за неделю.

Детали реализации:
- нужно придерживаться чистой архитектуры и структуры папок, созданной в проекте. Можно создавать новые файлы и папки
- нужно написать grpc интерфейс и http интерфейс (апи)
- нужно создать базу данных и положить в нее информацию из сети SUI чтобы каждый раз не собирать новую информацию и сэкономить коннекшены к ноде

Подробности про токен:

Токен минт происходит каждые 2 дня и выпущенные токены создаются в смарт контракте, пользователи сети могут выполнить функцию claim контракта, чтобы получить токены на свой кошелек.

Получить информацию об адресе контракта с функцией mint можно по адресу
https://app.chirpwireless.io/api/public/blockchain-info

Пример ответа:

***
{
"sui_testnet": "https://fullnode.testnet.sui.io:443",
"sui_mainnet": "https://fullnode.mainnet.sui.io:443",
"url": "https://fullnode.testnet.sui.io:443",
"url_main": "https://fullnode.mainnet.sui.io:443",
"chirpCurrency": "0x1ef4c0b20340b8c6a59438204467ca71e1e7cbe918526f9c2c6c5444517cd5ca::chirp::CHIRP",
"claimAddress": "0x769400ba81181f15a1b33ea011c332172aa7f8558e33c262932c2d84c4e69856::chirp::claim",
"nfts": [
{
"nftContractAddress": "0x9185af704124515bc588e71af96537a0e3a6d18aad56d7e0c2668cb1ddd5ca0d",
"nftModuleName": "skyward_soarer",
"nftTypeName": "SkywardSoarer"
},
{
"nftContractAddress": "0x3d33b1473b622e13231588478852d80adbfe0005e5326501c481af810ccbbaa8",
"nftModuleName": "kage_pass",
"nftTypeName": "KagePass"
}
],
"vaultId": "0xee375ee3186d35224da766e4a2cb8ab77d77bb5bef6a081cd11f294c3889ff8a",
"epoch": {
"unit": "hours",
"length": "48"
},
"current_network": "mainnet",
"dictionaryId": "0xdb7e4c349d5774c6b9e9b1607fa48816c7bdd57b34a8ff81d1efb1f242c10f59",
"vesting_ledger": "0xb305e415a3ef73da74ac5ffe4c0e0eee7b630c7882f4aedcfb5c6638a9d312a9",
"fountain_object_id": "0x4a726abecdc3d28aaeb55ae7878d3499ee8294fc2dc9b48c1d58935baf517a61",
"staking_package_id": "0x7b1bf8eed2e8a83dc83301240773f651e1eff9496264f9a17f1d08e78adf9bfb"
}
***

claimAddress - адрес контракта claim

chirpCurrency - адрес токена Chirp

Вот пример транзакции claim:

***
const tx = new Transaction();

tx.setGasBudget(DEFAULT_CLAIM_BUDGET);

const amountWithDecimals = convertAmountToBigNumber(amount)?.toString().split('.')?.[0];

tx.moveCall({
  target: blockchainInfo.claimAddress as `${string}::${string}::${string}`,
  arguments: [tx.object(blockchainInfo.vaultId), tx.pure.u64(amountWithDecimals)],
});

***

Далее пользователи либо продают токены, либо хранят их на кошельке, либо отправляют в стейкинг.

Для получения стейкинга используется вызов (пример на typescript)
suiClient.getOwnedObjects({
owner: address,
filter: {
StructType: `${packageID}::fountain_core::StakeProof<${coinType}, ${coinType}>`,
},
cursor,
options: { showContent: true },
}) as unknown as Promise<StakeProofsResponse>;
};

const packageID = blockchainInfo?.staking_package_id;
const coinType = blockchainInfo?.chirpCurrency;
const cursor = undefined; // stakingProofsData.hasNextPage ? stakingProofsData.nextCursor : undefined
const fountainObjectId = blockchainInfo?.fountain_object_id;

const resultDataFilteredByFountain = useMemo(() => {
return {
data: {
data:
result?.data?.data?.filter(
(proof) => proof.data?.content?.fields?.fountain_id.toLowerCase() === fountainObjectId?.toLocaleLowerCase()
) || [],
},
};
}, [fountainObjectId, result?.data?.data]);


## Реализованный функционал

### 1. **Доменные модели** ([internal/domain/transaction.go](cci:7://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/domain/transaction.go:0:0-0:0))
- [ChirpTransaction](cci:2://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/domain/transaction.go:7:0-18:1) - модель транзакции токена CHIRP
- [TokenStatistics](cci:2://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/domain/transaction.go:21:0-31:1) - статистика по токену
- [BlockchainInfo](cci:2://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/domain/transaction.go:34:0-42:1) - конфигурация блокчейна

### 2. **SUI Blockchain клиент** ([internal/infrastructure/blockchain/sui_client.go](cci:7://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/infrastructure/blockchain/sui_client.go:0:0-0:0))
- Подключение к SUI RPC
- Получение информации о блокчейне из Chirp API
- Запросы транзакций, чекпоинтов
- Парсинг данных блокчейна

### 3. **База данных** (`internal/infrastructure/storage/gormdb/`)
- Модель [ChirpTransactionModel](cci:2://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/infrastructure/storage/gormdb/transaction.go:14:0-27:1) с индексами
- Репозиторий с методами:
  - Создание транзакций (одиночных и батчами)
  - Получение по адресу, digest, временному диапазону
  - Расчет статистики (claimed, transferred, staked, bought, sold)
  - Подсчет уникальных держателей

### 4. **Сервисный слой** ([internal/service/transactions.go](cci:7://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/service/transactions.go:0:0-0:0))
- Синхронизация транзакций из блокчейна
- Определение типов транзакций (claim, transfer, stake, buy, sell)
- Получение статистики за неделю или произвольный период
- Обработка чекпоинтов батчами

### 5. **gRPC API** ([api/grpc/transactions.proto](cci:7://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/api/grpc/transactions.proto:0:0-0:0) + `internal/handler/grpc/`)
- [GetTransactionsByAddress](cci:1://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/handler/grpc/transactions.go:23:0-51:1) - транзакции по адресу
- [GetWeeklyStatistics](cci:1://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/handler/grpc/transactions.go:53:0-62:1) - статистика за неделю
- [GetStatistics](cci:1://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/handler/grpc/transactions.go:64:0-76:1) - статистика за период
- [SyncTransactions](cci:1://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/internal/handler/grpc/transactions.go:78:0-91:1) - запуск синхронизации

### 6. **HTTP REST API** (`internal/handler/http/`)
- `GET /api/v1/transactions/address` - транзакции по адресу
- `GET /api/v1/statistics/weekly` - недельная статистика
- `GET /api/v1/statistics` - статистика за период
- `POST /api/v1/sync` - синхронизация
- `GET /health` - health check

### 7. **Главное приложение** ([cmd/go-sui-test/main.go](cci:7://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/cmd/go-sui-test/main.go:0:0-0:0))
- Инициализация БД с автомиграцией
- Подключение к SUI mainnet
- Запуск gRPC и HTTP серверов
- Автоматическая синхронизация каждые 5 минут
- Graceful shutdown

### 8. **Конфигурация**
- Обновлен [config.yaml.example](cci:7://file:///c:/Users/kpozd/WebstormProjects/go-sui-test/app/config/config.yaml.example:0:0-0:0)
- Настройки для gRPC, HTTP, PostgreSQL

## Следующие шаги для запуска:

1. **Создать конфигурацию:**
```bash
cp app/config/config.yaml.example app/config/config.yaml
```

2. **Запустить PostgreSQL:**
```bash
docker-compose up -d
```

3. **Сгенерировать protobuf код:**
```bash
cd app
buf generate
```

4. **Запустить сервис:**
```bash
cd app
go run cmd/go-sui-test/main.go
```

Сервис будет собирать транзакции токена CHIRP из сети SUI, сохранять их в БД и предоставлять статистику через gRPC и HTTP API. Цель определения количества купленных/проданных токенов за неделю достигается через endpoint `/api/v1/statistics/weekly`.