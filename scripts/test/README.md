# Проверки обновления netlib

Контур использует отдельный Compose project `netlib-regression`, PostgreSQL 15/pgvector с тестовыми учётными данными и временным каталогом данных, настоящий Go-сервер и Nginx. Production-конфигурация Compose и `.env` не используются для подключения к БД тестов.

## Go и PostgreSQL

```sh
npm run test:server
```

Команда запускает `go test -race -tags=integration -count=1 ./...` в Go 1.25.1. Каждый integration-тест создаёт отдельную БД через реальные миграции и удаляет только её. Для самостоятельного запуска с Go нужен `NETLIB_TEST_DATABASE_URL=postgres://test:test@localhost:5432/netlib_test?sslmode=disable`; имя основной тестовой базы обязательно `netlib_test`. Без этого адреса integration-тесты завершаются ошибкой, а не пропускаются.

Проверяются снимки при одновременном входе, отказ входа без изменения комнаты, настройки/права, удалённый и отключённый лидер, независимая комната при блокировке, совместный timeout/election/join, request ID, ping, закрытие без лобби и инициирование соединения только с предыдущими участниками.

## Настоящие браузеры

```sh
docker build -t netlib:69212489-candidate .
docker compose -p netlib-regression -f scripts/test/compose.yml --profile runtime up -d netlib nginx
cd scripts/test
npm ci --ignore-scripts
npx playwright install chromium
cd ../..
node scripts/test/browser-smoke.cjs
```

Если Playwright уже установлен, вместо отдельной установки можно передать абсолютный путь к модулю через `PLAYWRIGHT_MODULE`. Выполненный локальный прогон использовал `D:/Projects/learn-chess/node_modules/@playwright/test` версии 1.59.1.

Тест загружает оригинальные npm-архивы `0.0.16` и `0.0.20` и проверяет заранее зафиксированный SHA-512. Архивы и JSON-результаты хранятся в игнорируемом `node_modules/.cache/netlib-regression/`. Основной CommonJS-бандл исполняется с настоящим eventemitter3 и `NODE_ENV=production`, как при сборке потребителя; legacy-бандл загружается напрямую. RTCPeerConnection и WebSocket не подменяются. Проверяются create/list/join, reliable/unreliable в обе стороны, номера каналов и ping по control-каналу. Chromium работает без окна и звука и закрывается в finally.

Для проверки рестарта установить `NETLIB_TEST_RESTART=1`. Скрипт перезапускает только сервис `netlib` в фиксированном тестовом проекте `netlib-regression`. URL можно задать через `NETLIB_TEST_SIGNALING_URL`, но разрешён только localhost. Runtime-сервер использует `ENV=production` внутри тестового контура: `ENV=test` намеренно сбрасывает детерминированный генератор peer ID при рестарте и не подходит для проверки сохранённой БД.

Для матрицы с другим локальным образом задать `NETLIB_TEST_IMAGE`, пересоздать только netlib и перечитать Nginx:

```sh
docker compose -p netlib-regression -f scripts/test/compose.yml --profile runtime up -d --no-deps --no-build netlib
docker compose -p netlib-regression -f scripts/test/compose.yml exec -T nginx nginx -t
docker compose -p netlib-regression -f scripts/test/compose.yml exec -T nginx nginx -s reload
node scripts/test/browser-smoke.cjs
```

Контур проверяет библиотечный транспорт. Он не заменяет сборку шахмат, проверку настоящего PeerNetwork.leave(), игровых ходов, рейтинга и активной партии. Эти проверки остаются в плане клиентского выпуска и в проверках реального потребителя перед production.

Отдельная проверка SC-05 запускается с `NETLIB_TEST_CLOSE_QUEUE=1`: она задерживает очередь оригинального клиента, ставит connect, закрывает Network и затем разрешает очередь. После закрытия новый Peer появляться не должен. Это детерминированная проверка клиентской границы отмены, а не замена полноценному игровому сценарию.

## Настоящие шахматы с текущим клиентом

При наличии checkout шахмат и его установленных зависимостей задать `NETLIB_CHESS_WORKSPACE` и запустить Playwright CLI из этого checkout:

```powershell
$env:NETLIB_CHESS_WORKSPACE = 'D:\Projects\learn-chess'
node "$env:NETLIB_CHESS_WORKSPACE/node_modules/@playwright/test/cli.js" test -c scripts/test/chess.config.cjs
```

Сигналинг из предыдущего раздела должен уже работать. Контур запускает Vite на 127.0.0.1:3196 и использует существующий сценарий приглашения/двух ходов шахмат с реальной доской, адаптером и оригинальным 0.0.16. Адрес задаётся ранним флагом SDK. Данные игроков обслуживаются существующим ApiMock; внешние WebSocket и изменяющие HTTP-запросы заблокированы, локальный сигналинг и WebRTC не подменяются. Браузеры без звука закрываются после каждого сценария.

Проверяются ожидание соперника, подключение и активная партия. Для воспроизводимого рестарта во время подключения тест удерживает первый вызов настоящего setLocalDescription до восстановления сигналинга. В активной партии рестарт выполняется после доставки первого хода обоим игрокам; далее проверяется второй ход. Исходники шахмат не изменяются.

Эта диагностическая проверка не входит в `test:server`: её отрицательный результат обозначает ограничение восстановления потребителя и не скрывается через skip/expected-failure. Результаты и trace сохраняются в игнорируемом node_modules/.cache/netlib-regression/chess-results. Для сравнения со старым образом сначала применить его в тестовом Compose, затем установить NETLIB_CHESS_BASELINE=1 — артефакты попадут в отдельный chess-baseline.

## Завершение

```sh
docker compose -p netlib-regression -f scripts/test/compose.yml --profile runtime down
```

Команда относится только к тестовому проекту; временная БД исчезает при удалении её контейнера. Кэши Go и тестовый том сертификатов сохраняются. Команды остановки и удаления не запускаются автоматически из браузерного теста.
