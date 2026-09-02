# Анализ обновлений netlib — карточка 69212489

Дата проверки: 2 сентября 2026 года.

Уточнение после проверки потребителя: пользователь утвердил план v3 «наш серверный форк + оригинальный клиент @poki/netlib 0.0.20». Шахматы сейчас используют оригинальный 0.0.16. Решение о выпуске и обязательные проверки находятся в [OpenSpec](../openspec/changes/update-netlib-stability/proposal.md); рекомендации ниже сохраняют контекст исходного исследования.

**Полезные обновления есть. Рекомендуется выборочно перенести исправления подключения и серверной стабильности. Полное обновление затрагивает клиент, схему БД, зависимости и удалённую в форке инфраструктуру тестирования.**

## Состояние репозиториев

| Что | Проверенное состояние |
| --- | --- |
| Наш форк | [webdeveryorg-wq/netlib](https://github.com/webdeveryorg-wq/netlib), main: `74f579995e2dd240423cbd73386485b669dd359d`, 29.05.2026 |
| Оригинал | [poki/netlib](https://github.com/poki/netlib), main: `44c9d0746799c51bf9c652c57162e8b273ed3901`, 28.07.2026 |
| Общий предок | `aa2449a3a8fc8c5eefcece25b630865cf8fdff0d`, 26.01.2026 |
| Первый собственный коммит | `1c32a88`, 29.01.2026 |
| Расхождение | 13 собственных коммитов и 56 коммитов upstream: 38 от Dependabot, 18 остальных, включая две смены версии |
| Пакет форка | `@smartberry/netlib`, версия в package.json: `0.0.19` |
| Опубликованный оригинал | `@poki/netlib@0.0.20`, опубликован 17.06.2026, текущий npm latest |

Локальный HEAD совпадает с текущим origin/main. Свежие вершины проверены через git ls-remote; история загружена напрямую из GitHub, общий предок вычислен через git merge-base. Кэшированная веб-страница показывала только мартовские изменения, поэтому полнота проверялась по Git.

Оригинальная 0.0.19 опубликована 08.05.2026, а номер 0.0.19 у нас появился в январе. Совпадение номера **не означает одинаковый код**. Даты проверены по [реестру npm](https://registry.npmjs.org/@poki%2fnetlib). В оригинальном репозитории на дату проверки нет Git-тегов и GitHub Releases; ориентиры — коммиты и npm.

Развёрнутый сервер и версии библиотеки в играх не проверялись. Дата фактического запуска сервера не установлена: отчёт относится к текущему исходному коду форка, а не к подтверждённому production SHA.

## Что стоит перенести

| Приоритет | Изменение | Польза | Область |
| --- | --- | --- | --- |
| Высокий | [#364](https://github.com/poki/netlib/pull/364), `f4fc5c3`, 17.06: гонки при подключении | Устраняет несколько причин нестабильного соединения при одновременном входе игроков и получении TURN credentials | Сервер и клиент; адаптация наших изменений Peer |
| Высокий | [#340](https://github.com/poki/netlib/pull/340), `5d249f7`, 27.03: выбор лидера | Убирает эксклюзивную блокировку всей таблицы peers; выбирает лидера только из существующих подключённых участников | Сервер |
| Высокий | [#344](https://github.com/poki/netlib/pull/344), `0dcb04a`, 30.03: гонка контекста | Запрос перестаёт перезаписывать контекст, одновременно используемый ping-горутиною | Сервер |
| Средний, малый объём | [#388](https://github.com/poki/netlib/pull/388), `44c9d07`, 28.07: отключение до входа в лобби | Убирает ненужный выбор лидера и ошибку в логах для игрока без лобби | Сервер |
| По использованию | [#355](https://github.com/poki/netlib/pull/355), `f04b0d4`, 07.05: legacy.js | Исправляет сборку вложенного async-кода переговоров WebRTC | Клиент, если используется legacy-бандл |
| Дополнительное улучшение | [#337](https://github.com/poki/netlib/pull/337), `04206a7`, 24.03: maxMessageSize | Даёт согласованный предел размера сообщения; полезно для больших снимков состояния | Клиент; getter peer.maxMessageSize, автоматического разбиения сообщений нет |

### Главный кандидат: #364

В форке сообщения сигналинга обрабатываются асинхронно без общей последовательной очереди. Исправление upstream:

- обрабатывает SDP и ICE в порядке поступления;
- откладывает ICE-кандидаты до установки remote description;
- игнорирует устаревшие SDP answer;
- предотвращает повторное создание Peer во время ожидания TURN credentials;
- на сервере возвращает из JoinLobby список участников до присоединения и рассылает запросы соединения по этому снимку.

Ответы на запрос credentials намеренно обходят очередь: иначе обработчик connect может ждать ответ, стоящий за ним в той же очереди. При переносе это исключение необходимо сохранить.

Серверная часть меняет внутренний Go-интерфейс Store.JoinLobby, но не формат пакетов протокола и не схему БД. Её можно перенести отдельно. Для всего набора исправлений нужна также новая версия клиентской библиотеки и её обновление в играх; перезапуска сервера недостаточно.

Частота этих проблем у нас не измерялась. Автор upstream указал, что регрессионные тесты гонок в PR не добавлены. Перед выпуском нужна проверка одновременных подключений и переподключений.

## Что рассматривать отдельно

### Новая оценка задержки: #325 и #326

[#325](https://github.com/poki/netlib/pull/325) удаляет эксперименты с latency-векторами и географическими координатами. [#326](https://github.com/poki/netlib/pull/326) вводит оценку по стране/региону и таблице измерений Poki примерно на 50 тысяч строк.

Полезный побочный эффект: из клиента исчезают 11 × 3 HTTP-запроса к ping-серверам Poki и ожидание их результатов перед hello. В текущем коде это ожидание есть. Фактическое ускорение не измерялось.

Цена переноса:

- миграция 1769954921_remove_latency удаляет latency_vector, geo, связанные функции и расширения БД;
- миграция 1770020948_latency3 добавляет country, region, latencies, latency_meta и новую функцию оценки;
- поле latency2 исчезает из списка лобби, смысл latency меняется, настройка testLatency удаляется;
- без страны новая оценка возвращает запасное значение 250 мс — это не измеренный ping;
- нужно пересмотреть нашу предварительную инициализацию vector в internal/signaling/stores/setup.go.

В конфигурации Nginx репозитория не найдено формирования CF-IPCountry / X-Geo-Region. Они могут поступать от внешнего прокси; его настройки не проверялись. Без источника геоданных польза нового подбора лобби ограничена. Оценка не снижает сама по себе задержку установленного P2P-соединения.

Рекомендация: не включать эти миграции в первый пакет исправлений стабильности. При необходимости выделить отдельную работу с проверкой API и старых клиентов. После удаления колонок откат одного бинарника к старой версии недостаточен.

### Зависимости

| Компонент | Форк | Upstream |
| --- | --- | --- |
| Go в go.mod | 1.24.0 | 1.26.5 |
| Go в Docker builder | 1.25.1 | 1.26.5 |
| coder/websocket | 1.8.14 | 1.8.15 |
| pgx/v5 | 5.8.0 | 5.10.0 |
| mongodb-filter-to-postgres | 1.0.7 | 1.0.8 |
| x/crypto | 0.47.0 | 0.54.0 |
| zap | 1.27.1 | 1.28.0 |
| Parcel | 2.16.3 | 2.16.4 |
| TypeScript | 5.9.3 | 6.0.3 |
| @roamhq/wrtc | 0.9.1 | 0.10.0 |
| ws | 8.19.0 | 8.21.0 |
| eventemitter3 | 5.0.4 | 5.0.4 |

Обновления Go/toolchain и зависимостей стоит выполнить отдельным этапом. TypeScript меняет основную версию; нужны проверка типов и сборка используемых бандлов. ws и wrtc находятся в devDependencies и не являются WebSocket-реализацией Go-сервера.

[Релиз coder/websocket 1.8.15](https://github.com/coder/websocket/releases/tag/v1.8.15) содержит исправление отправки сжатого сообщения одним фреймом, сокращение выделений памяти при чтении и исключение лишних timeout-callback для фоновых контекстов. Применимость отдельных оптимизаций зависит от настроек соединения.

Сам факт обновления Dependabot не доказывает наличие эксплуатируемой уязвимости в нашем сервере. Аудит достижимости уязвимостей не выполнялся.

### Низкий приоритет

- [#354](https://github.com/poki/netlib/pull/354): дополнительные события latency на 25, 50, 75 и 100 секундах, помимо 10-й. Полезно для аналитики, соединение само по себе не улучшает.
- [#351](https://github.com/poki/netlib/pull/351): Go lint и CI. Стоит адаптировать при восстановлении проверок. У нас удалены workflows и features, а Cucumber отсутствует в зависимостях при оставшемся npm-script cucumber.
- [#327](https://github.com/poki/netlib/pull/327): ссылка на Defold — документация.
- SOPS, GitHub Actions, группировка Dependabot и Slack-уведомления PR: конфигурация upstream, без прямой пользы для собственного сервера.

Открытые PR на дату проверки: [#390](https://github.com/poki/netlib/pull/390) (образ Go 1.26.6), [#389](https://github.com/poki/netlib/pull/389) (x/crypto 0.55.0), [#386](https://github.com/poki/netlib/pull/386) (npm), [#349](https://github.com/poki/netlib/pull/349) (переработка тестов), [#245](https://github.com/poki/netlib/pull/245) (типы rtcerror). Они ещё не входят в main и не включены в 56 завершённых коммитов.

## Что у нас уже есть и что сохранить

Сжатие PostgreSQL NOTIFY/LISTEN (#311–315), исправления запросов координат (#304–306), подавление ожидаемых connection reset by peer (#317) вошли ещё в декабре 2025. Сортировка и лимиты списка лобби (#282) — в сентябре 2025. Это не новые кандидаты на перенос.

Собственные отличия форка:

- пакет @smartberry/netlib, Docker Compose, Nginx, SSL/certbot, лимиты и ротация логов;
- STUN fallback без Cloudflare App ID;
- создание vector перед миграциями при DATABASE_URL;
- преобразование ICE-кандидатов для localhost и поддержка RTCIceCandidateInit;
- явные createOffer/createAnswer и очистка negotiation-таймера;
- сортировка имён data channel перед назначением ID. При смешивании с оригинальным клиентом нужно проверить порядок каналов, особенно пользовательских.

## Проверка переноса

Пробное слияние через git merge-tree выполнено без изменения ветки, индекса и рабочих файлов. **Конфликты в 9 файлах:**

- .github/dependabot.yml;
- .github/workflows/build.yaml;
- docs/api-reference.md;
- docs/basic-usage.md;
- features/support/steps/network.ts;
- features/support/world.ts;
- lib/peer.ts;
- package.json;
- yarn.lock.

Первые шесть удалены в форке и изменены upstream. Остальные содержат конфликты содержимого.

Обнаружены также проблемы, которые Git не отмечает отдельным конфликтом:

1. В автоматически слитом internal/signaling/stores/setup.go остаётся наш вызов pgx.ConnectConfig, но upstream удаляет импорт pgx. Полученный код не компилируется; это установлено чтением дерева, Go-сборка не запускалась.
2. Новая очередь RTCIceCandidate[] несовместима с нашим CandidatePacket.candidate, допускающим RTCIceCandidateInit. Минимальная проверка установленным TypeScript подтвердила TS2345. Очередь и метод добавления кандидатов нужно адаптировать под этот тип.

Отдельные переносы #340, #344 и #388 проверены через merge-tree с родителями соответствующих коммитов в качестве базы: каждый по отдельности применяется к текущему форку без текстовых конфликтов. Это не интеграционный тест их совместной работы.

## Рекомендуемый порядок

1. Перенести серверные #340, #344 и #388 без изменения схемы БД.
2. Адаптировать сервер и клиент из #364 с сохранением наших отличий Peer. При использовании legacy.js включить #355; #337 можно добавить отдельным небольшим изменением.
3. Проверить одновременный вход нескольких клиентов, отключение лидера, уход до входа в лобби, reconnect, TURN credentials и пользовательские data channels. Выполнить серверную сборку, проверку с race detector и клиентскую проверку типов/бандлов.
4. Выпустить новую версию @smartberry/netlib, обновить её в играх, собрать и развернуть сервер. До выкладки сверить production SHA и версии клиентов.
5. Отдельно обновить Go/зависимости. Миграцию latency выполнять после решения о необходимости этой функции.

**Текущий npm run server:restart опасен для обычного перезапуска:** он выполняет `docker compose down -v && npm run server:start`. Флаг -v удаляет объявленные тома postgres_data, certbot_certs и certbot_www, то есть может удалить PostgreSQL и сертификаты. Нужен способ пересоздания сервиса с сохранением томов, подобранный под фактическую конфигурацию развёртывания.

## Границы проверки

Проверены карточка и комментарий, свежие ветки, история, diff исходников, npm-версии, открытые PR и пробные слияния. Типы ICE проверены минимальным примером через TypeScript. Интеграционные тесты и Go-сборка не запускались; Go отсутствует в PATH. Production не проверялся.

Создан только отчёт. Код, ветка и индекс не изменены; сервер не перезапускался; сообщения в карточку не отправлялись.

## Все 56 новых коммитов upstream

Далее исходные названия коммитов без перевода; таблица сформирована из проверенной Git-истории.

| Дата | Коммит | Изменение |
| --- | --- | --- |
| 2026-02-06 | [3dff90b](https://github.com/poki/netlib/commit/3dff90b) | Remove latency 1 and 2 experiments (#325) |
| 2026-02-06 | [676ea6a](https://github.com/poki/netlib/commit/676ea6a) | New latency experiment (#326) |
| 2026-02-06 | [4648097](https://github.com/poki/netlib/commit/4648097) | Bump @cucumber/cucumber from 12.5.0 to 12.6.0 in the npm group (#324) |
| 2026-02-09 | [a5bd77e](https://github.com/poki/netlib/commit/a5bd77e) | Add a link to the Defold engine integration (#327) |
| 2026-02-09 | [0353d8a](https://github.com/poki/netlib/commit/0353d8a) | Bump the npm group with 3 updates (#328) |
| 2026-02-16 | [92a8bc1](https://github.com/poki/netlib/commit/92a8bc1) | Bump golang.org/x/crypto from 0.47.0 to 0.48.0 in the gomod group (#329) |
| 2026-03-06 | [3fd46ab](https://github.com/poki/netlib/commit/3fd46ab) | Bump github.com/docker/cli from 27.4.1+incompatible to 29.2.0+incompatible (#330) |
| 2026-03-09 | [9f61647](https://github.com/poki/netlib/commit/9f61647) | Bump @cucumber/cucumber from 12.6.0 to 12.7.0 in the npm group (#331) |
| 2026-03-16 | [95ef5b2](https://github.com/poki/netlib/commit/95ef5b2) | Bump the github-actions group with 2 updates (#333) |
| 2026-03-16 | [d597dd6](https://github.com/poki/netlib/commit/d597dd6) | Bump @roamhq/wrtc from 0.9.1 to 0.10.0 in the npm group (#332) |
| 2026-03-20 | [7c088eb](https://github.com/poki/netlib/commit/7c088eb) | Bump flatted from 3.2.7 to 3.4.2 (#334) |
| 2026-03-24 | [9f57d2b](https://github.com/poki/netlib/commit/9f57d2b) | Switch to Go 1.25 and update golang.org/x/crypto (#338) |
| 2026-03-24 | [99c0196](https://github.com/poki/netlib/commit/99c0196) | Update sops to v3.12.2 (#336) |
| 2026-03-24 | [04206a7](https://github.com/poki/netlib/commit/04206a7) | Add Peer.maxMessageSize() (#337) |
| 2026-03-26 | [5d531c0](https://github.com/poki/netlib/commit/5d531c0) | Bump picomatch from 2.3.1 to 2.3.2 (#339) |
| 2026-03-27 | [5d249f7](https://github.com/poki/netlib/commit/5d249f7) | Improve leader election (#340) |
| 2026-03-30 | [113f65b](https://github.com/poki/netlib/commit/113f65b) | Bump github.com/jackc/pgx/v5 from 5.8.0 to 5.9.1 in the gomod group (#342) |
| 2026-03-30 | [11b3bd2](https://github.com/poki/netlib/commit/11b3bd2) | Bump the npm group with 2 updates (#343) |
| 2026-03-30 | [0dcb04a](https://github.com/poki/netlib/commit/0dcb04a) | Fix context race condition (#344) |
| 2026-04-20 | [4c50ba5](https://github.com/poki/netlib/commit/4c50ba5) | Bump @cucumber/cucumber from 12.7.0 to 12.8.1 in the npm group (#346) |
| 2026-04-20 | [05dfa51](https://github.com/poki/netlib/commit/05dfa51) | Bump golang.org/x/crypto from 0.49.0 to 0.50.0 in the gomod group (#345) |
| 2026-04-23 | [8aaf9b8](https://github.com/poki/netlib/commit/8aaf9b8) | Bump github.com/jackc/pgx/v5 from 5.9.1 to 5.9.2 (#347) |
| 2026-04-25 | [217287d](https://github.com/poki/netlib/commit/217287d) | Bump go.opentelemetry.io/otel from 1.37.0 to 1.41.0 (#348) |
| 2026-04-29 | [f920e40](https://github.com/poki/netlib/commit/f920e40) | Fix golangci-lint and add as action (#351) |
| 2026-05-04 | [f238285](https://github.com/poki/netlib/commit/f238285) | Bump the npm group across 1 directory with 2 updates (#353) |
| 2026-05-04 | [00c3f54](https://github.com/poki/netlib/commit/00c3f54) | Bump the gomod group with 2 updates (#352) |
| 2026-05-05 | [d80a4ce](https://github.com/poki/netlib/commit/d80a4ce) | Emit more avg-latency-at- events (#354) |
| 2026-05-07 | [f04b0d4](https://github.com/poki/netlib/commit/f04b0d4) | Extract negotiation code to fix broken legacy bundle (#355) |
| 2026-05-08 | [91e966c](https://github.com/poki/netlib/commit/91e966c) | v0.0.19 |
| 2026-05-21 | [2146659](https://github.com/poki/netlib/commit/2146659) | Bump the npm group with 2 updates (#357) |
| 2026-05-21 | [4f2bbff](https://github.com/poki/netlib/commit/4f2bbff) | Bump golang.org/x/crypto from 0.50.0 to 0.51.0 in the gomod group (#356) |
| 2026-05-25 | [93370c4](https://github.com/poki/netlib/commit/93370c4) | Bump the github-actions group with 2 updates (#360) |
| 2026-06-01 | [8bddbc4](https://github.com/poki/netlib/commit/8bddbc4) | Bump the github-actions group with 2 updates (#363) |
| 2026-06-01 | [a029761](https://github.com/poki/netlib/commit/a029761) | Bump the npm group across 1 directory with 2 updates (#362) |
| 2026-06-01 | [a8a41e8](https://github.com/poki/netlib/commit/a8a41e8) | Bump golang.org/x/crypto from 0.51.0 to 0.52.0 in the gomod group (#361) |
| 2026-06-01 | [be625f6](https://github.com/poki/netlib/commit/be625f6) | Bump golang from 1.25.1-alpine to 1.26.3-alpine in the docker group across 1 directory (#359) |
| 2026-06-08 | [0946d40](https://github.com/poki/netlib/commit/0946d40) | Bump golang from 1.26.3-alpine to 1.26.4-alpine in the docker group (#365) |
| 2026-06-15 | [0abf20f](https://github.com/poki/netlib/commit/0abf20f) | Bump the gomod group with 2 updates (#367) |
| 2026-06-15 | [3dd7e61](https://github.com/poki/netlib/commit/3dd7e61) | Bump golang from `f23e8b2` to `7a3e500` in the docker group (#368) |
| 2026-06-15 | [9304cd2](https://github.com/poki/netlib/commit/9304cd2) | Bump @cucumber/cucumber from 12.9.0 to 13.0.0 in the npm group across 1 directory (#366) |
| 2026-06-15 | [bb62df5](https://github.com/poki/netlib/commit/bb62df5) | Bump minimatch from 3.1.2 to 3.1.5 (#369) |
| 2026-06-17 | [f4fc5c3](https://github.com/poki/netlib/commit/f4fc5c3) | Fix signaling races during peer connection setup (#364) |
| 2026-06-17 | [ade2122](https://github.com/poki/netlib/commit/ade2122) | v0.0.20 |
| 2026-06-22 | [7a4c0e5](https://github.com/poki/netlib/commit/7a4c0e5) | Bump actions/checkout from 6 to 7 in the github-actions group (#374) |
| 2026-06-22 | [fd946bb](https://github.com/poki/netlib/commit/fd946bb) | Bump golang from `7a3e500` to `3ad5730` in the docker group (#373) |
| 2026-06-22 | [7fd1979](https://github.com/poki/netlib/commit/7fd1979) | Bump github.com/coder/websocket from 1.8.14 to 1.8.15 in the gomod group (#372) |
| 2026-06-29 | [49c778a](https://github.com/poki/netlib/commit/49c778a) | Group security dependabot updates (#375) |
| 2026-06-30 | [edf9b18](https://github.com/poki/netlib/commit/edf9b18) | Bump github.com/opencontainers/runc from 1.2.8 to 1.3.6 (#376) |
| 2026-07-09 | [bb9bb1e](https://github.com/poki/netlib/commit/bb9bb1e) | Upgrade Go to 1.26.5 (#380) |
| 2026-07-09 | [5ce8a14](https://github.com/poki/netlib/commit/5ce8a14) | Add Slack PR notification (#382) |
| 2026-07-10 | [2651723](https://github.com/poki/netlib/commit/2651723) | Bump the npm-security group across 1 directory with 2 updates (#381) |
| 2026-07-10 | [e5126ee](https://github.com/poki/netlib/commit/e5126ee) | Bump the github-actions group across 1 directory with 2 updates (#379) |
| 2026-07-22 | [360ec5e](https://github.com/poki/netlib/commit/360ec5e) | Bump golang.org/x/crypto from 0.53.0 to 0.54.0 in the gomod group (#383) |
| 2026-07-22 | [2512f90](https://github.com/poki/netlib/commit/2512f90) | Bump brace-expansion from 1.1.11 to 1.1.16 in the npm-security group across 1 directory (#385) |
| 2026-07-27 | [a291a3c](https://github.com/poki/netlib/commit/a291a3c) | Bump actions/setup-go from 6 to 7 in the github-actions group (#387) |
| 2026-07-28 | [44c9d07](https://github.com/poki/netlib/commit/44c9d07) | Don't try to elect a leader when there is no lobby (#388) |
