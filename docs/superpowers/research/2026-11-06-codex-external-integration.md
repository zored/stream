# Codex Desktop: внешние интерфейсы для `zored_stream`

Дата: 2026-11-06. Продолжение задачи 1 из [плана](../plans/2026-11-06-codex-desktop.md).
Метод: только публичные источники — документация learn.chatgpt.com, исходники `openai/codex`, issues, открытые интеграции.
Установленный Codex, `~/.codex`, процессы и сокеты не исследовались; сторонний код не запускался; второй app-server не поднимался.

Пометки: **прочитано** — видно в источнике, на машине не проверялось; **сообщение пользователя** — комментарий в issue без ответа OpenAI; **не проверено** — нет ни источника, ни наблюдения.
Ничего из этого документа на этой машине не наблюдалось, поэтому пометки «проверено» нет.

## Версии

| Что | Значение |
|---|---|
| Приложение (снимок прошлого исследования) | ChatGPT, bundle `com.openai.codex`, `26.1103.31037` |
| Отдельный CLI | `codex-cli 0.153.2` |
| `openai/codex` main | `ebe94e25c5930fd6154490e50155f7f88a4a398c` (2026-11-06) |
| Тег `rust-v0.153.2` | `657a993cbee87acf52d14b758ce49dbd46d1b8eb` |
| CLI внутри Desktop 26.1103 | неизвестен; соседние сборки 26.1027 несут `0.166.0-alpha.5.1` (#58495) — предположение |

На main README app-server сокращён до списка изменений; полный протокол лежит в README тега 0.153.2 и на https://learn.chatgpt.com/docs/app-server.
Контракт CLI 0.153.2 и контракт app-server внутри Desktop — разные версии; ниже метод помечен версией, где он найден.

## Главный ответ

1. **Публичного способа подключиться к app-server самого Desktop нет.** Нет endpoint, файла порта или правила discovery. По сообщениям пользователей (26.820–26.1027, CLI до 0.166) Desktop на macOS запускает свой app-server дочерним процессом по stdio с `-c mcp_servers.codex_app=…`; control socket `~/.codex/app-server-control/app-server-control.sock` при этом отсутствует (#25914, #40134, #41014, #58495). OpenAI ни в одном из этих issues не ответил.
2. **Отдельный app-server не видит живое состояние Desktop.** Документация: только один процесс держит поток на запись, `thread/resume`/`archive`/`delete` из другого процесса дают `-32600` (README@0.153.2:439). Запросы `item/tool/requestUserInput` и approvals уходят только подписчикам процесса-владельца (`outgoing_message.rs:388-523`). Для чужого процесса потоки Desktop — `notLoaded`. Обратный риск: если Desktop поток не держит, `thread/resume` из нашего процесса забирает writer-lock, и Desktop показывает «open in another app» (#41014, сообщение пользователя).
3. **Ни одна открытая интеграция к Desktop не подключается.** Все поднимают свой app-server или daemon и добирают живое состояние частными файлами, UI-автоматизацией или инъекцией (CDP, Node inspector).
4. Итог по задачам: из четырёх функций без внутренностей Codex и без UI-автоматизации делается только **адресное открытие чата** и **каталог чатов из сохранённой истории**. Живые статусы, текущий чат, смена model/effort у открытого чата и ответы на вопросы требуют изменения условий.

## Функции

### 1. Общий тайл чатов

| Поле | Источник | Класс | Чтение/запись | Версия | Доказательство | Не проверено |
|---|---|---|---|---|---|---|
| список, `id`, `name` (заголовок), `cwd`, `parentThreadId`, `forkedFromId`, `model`, `reasoningEffort` | `thread/list` (`archived`, `cwd`, `sourceKinds`, `sortKey`, cursor), `thread/read {threadId}` без подписки | официальный контракт | чтение | 0.153.2 и main | learn «app-server»; `v2/thread_data.rs` main:205-300; `common.rs` main:814 / tag:700 | что свой app-server видит все чаты Desktop, включая созданные в 26.1103 |
| архив | `thread/list {archived:true}`; события `thread/archived` / `thread/unarchived` только своего процесса | официальный | чтение | обе | README@0.153.2 | флага на объекте нет |
| статус работает / ждёт / простаивает | `ThreadStatus`: `notLoaded`, `idle`, `systemError`, `active{activeFlags:[waitingOnApproval, waitingOnUserInput]}`, push `thread/status/changed` | официальный, **только для потоков своего процесса** | чтение | обе | `thread.rs` main:1729-1746; `thread_status.rs`:437-451 | для потоков Desktop из чужого процесса — всегда `notLoaded` (#25914, #36571, сообщения пользователей) |
| завершён | `turn/completed` (`completed` / `interrupted` / `failed`) | официальный, только свой процесс | чтение | обе | `v2/turn.rs`:31-38 | то же |
| непрочитан | `readState` в `thread/read`, `readStates` в `thread/list`, `thread/readState/changed` | экспериментальный, **только main, нет в 0.153.2** | чтение | main | README@main:70-111 | совпадает ли с синими точками Desktop; есть ли в CLI, вшитом в 26.1103 |
| текущий видимый чат | — | **сигнала нет** | — | — | в протоколе метода нет | — |
| открыть чат | `codex://threads/<id>`; новый: `codex://new?prompt=&path=` (prompt подставляется, не отправляется) | официальный deep link | изменение UI | документирован сейчас | learn «Reference / Deep links»; #21779, #30895 | окно выбрать нельзя (#29605); переход ~1 с (#45011, Linux); read-back перехода нет |

Частные, публично описанные механизмы (используют аналоги, не официальный контракт; для нас — только при изменении условий, см. ниже):

- `~/.codex/state_5.sqlite`, таблица `threads` — список, архив, родитель (`deck-threads` `electron/codex-state.cjs:293-307`, `twidtwid` `src/lib/codex-store.ts:199-233`). Статуса и pid в таблице нет (#36571).
- rollout JSONL: `event_msg` `task_started` / `task_complete` / `turn_aborted`, открытый `function_call` `request_user_input` без `function_call_output` — работает, ждёт ответа, завершён без окна по времени (`ckundel2008` `src/agent-titles.js:161-200`, `DEXCERRONE` `bridge/src/state.mjs:37-95`).
- `~/.codex/.codex-global-state.json` — непрочитанные в двух несовместимых форматах: `unread-thread-ids-by-host-v1` и `electron-thread-read-state-v1.unreadByIdentity` (`ckundel2008` `src/thread-read-state.js:26-40`; при неоднозначности отдаёт «неизвестно»).
- Лог Desktop `~/Library/Logs/com.openai.codex/YYYY/MM/DD/*.log`, строка `thread_stream_view_activity_changed … active=true … rendererWindowFocused=true conversationId=<uuid>` — текущий чат (`twidtwid` `src/lib/desktop-active.ts:40-70`).
- Эвристики, которые нам запрещены спецификацией: «работает, если активность моложе 30 мин» (`deck-threads` `ACTIVE_WINDOW_SECONDS`), «непрочитан, если `task_complete` новее моего readAt» (`DEXCERRONE`, `twidtwid`), NLP-классификатор последнего сообщения (`CodexDeck` `service.ts:174-205`).

Восстановление после рестарта демона: официальный путь (`thread/resume` повторяет висящие запросы, `outgoing_message.rs:525-561`) работает только внутри процесса-владельца. Для Desktop — только через частные файлы выше.

### 2. Модель и effort

| Что | Метод | Класс | Граница | Доказательство | Не проверено |
|---|---|---|---|---|---|
| каталог и доступные effort | `model/list` → `Model{id, displayName, hidden, isDefault, supportedReasoningEfforts[], defaultReasoningEffort}` | официальный | чтение; аккаунт тот же, что у Desktop | learn «List models»; `v2/model.rs`:54-153 | есть ли `6.1 Terra` / `6.1 Sol` и у каких effort |
| текущее значение чата | `Thread.model`, `Thread.reasoningEffort` из `thread/read` | официальный | для незагруженного потока — последнее сохранённое значение | README@0.153.2:357-363 | совпадает ли с пикером Desktop, пока поток открыт в нём |
| новый чат | `thread/start {model, effort}` | официальный | только для потока своего процесса; deep link `codex://new` model/effort не принимает | learn «Turns»; #29605 | — |
| существующий чат, следующий ход | `turn/start {model, effort}` (становятся defaults потока); `thread/settings/update {threadId, model, effort}` с ответом accept/reject (#47937) | `turn/start` — официальный; `thread/settings/update` — экспериментальный, есть в 0.153.2 (`common.rs` tag:633) | **требует загрузить поток в свой процесс**; для чата Desktop — `-32600` | README@0.153.2:192, 207; `v2/thread.rs` main:236-330 | применение к потоку Desktop не проверено ни одним проектом; `twidtwid` держит такой вызов в мёртвом коде (`src/lib/app-server.ts:72-87`) |
| активный ход | `turn/settings/update {threadId, turnId, model, effort}` → `applied` / `targetUnavailable` | экспериментальный, нужна выключенная по умолчанию фича `step_model_switching`; будущие ходы не меняет | только свой процесс | README@0.153.2:1366-1416 | — |
| defaults | `config/value/write`, `model`, `model_reasoning_effort` в `config.toml` | официальный | только будущие чаты; существующие не перечитываются (README@0.153.2:306); глобальный побочный эффект, запрещён S3 без согласия | — | перечитывает ли Desktop конфиг без рестарта |

Аналоги меняют effort Desktop только UI-автоматизацией: ⌘⌥E через System Events (`CodexDeck` `service.ts:888-931`), AX-пикер с read-back (`twidtwid` `native/ComposerControls.swift:1008-1042`, `main.swift:571-660`), хоткеи после правки `~/.codex/keybindings.json` и рестарта Codex (`DEXCERRONE` `scripts/setup.mjs:10-55`), CDP или Node inspector (`threaddeck` `src/micro-main-inspector.js`). Всё это нам запрещено.

### 3. Ответы на вопросы кнопками 1–8

- Контракт: server→client `item/tool/requestUserInput {threadId, turnId, itemId, questions[], isBlocking}`; вопрос `{id, header, question, isOther, isSecret, options?: [{label, description}]}`, 1–3 вопроса. Ответ `{answers: {<questionId>: {answers: [string]}}}`; вариант передаётся label, свободный текст — строкой. Экспериментальный, документирован в learn «Approvals» (`v2/item.rs` main:1855-1940).
- Пока вопрос висит: `activeFlags: ["waitingOnUserInput"]`. После ответа или отмены — `serverRequest/resolved {threadId, requestId}`. Несколько клиентов одного процесса получают запрос, первый ответ выигрывает (`outgoing_message.rs:657-686`). Подходит под F2: release привязан к захваченному press, исчезновение вопроса видно по `serverRequest/resolved`.
- Ловушка: ошибка JSON-RPC или нераспознанный ответ молча превращаются в **пустой ответ** (`bespoke_event_handling.rs:1735-1775`). Ответ нужно валидировать до отправки.
- `requestId` живёт внутри процесса; отдельного метода «список висящих вопросов» нет, повтор только через `thread/resume` загруженного потока.
- **Для чата Desktop недоступно**: запрос получают только подписчики процесса-владельца. Единственный путь, о котором сообщают пользователи, — Remote Control через облачный relay с QR-сопряжением (Rope Notes, #25914). Это не документированный API для сторонних клиентов, и ответ из него может оставлять карточку в Desktop висящей (#53489).

### 4. Сохранение Claude

Конфликтов нет: всё найденное — отдельный адаптер. Порт и маршрутизация по приложению в фокусе остаются как в `ai_darwin.go` и `choice_darwin.go`.

## Три пути

| Путь | Что даёт | Чего не даёт | Статус |
|---|---|---|---|
| **A. Публичный контракт к процессу Desktop** | всё: статусы, вопросы, model/effort с read-back, восстановление | — | **закрыт.** Endpoint не опубликован; `CODEX_APP_SERVER_USE_LOCAL_DAEMON=1` недокументирован и сломан с 26.820 (#41014, подтверждено до 0.164); даже при подключении ломаются инструменты `codex_app` (#47425, 26.1027). Feature request #40134 открыт, ответа OpenAI нет |
| **B. Ограниченная поддержка разрешёнными внешними механизмами** | deep link `codex://threads/<id>`; каталог (id, заголовок, cwd, родитель, архив, сохранённые model/effort) через `thread/list` / `thread/read` своего app-server или read-only `state_5.sqlite`; `model/list` | живой статус, ожидание ответа, непрочитанность в 0.153.2, текущий чат, смена model/effort открытого чата, ответы на вопросы | возможен частично. Статусы и непрочитанность — только через частные файлы (rollout, global-state, лог Desktop), что требует отдельного согласия |
| **C. Собственный клиент app-server** | полный контракт для **своих** чатов: статусы, `requestUserInput`, model/effort | чаты Desktop: созданные нами не видны в сайдбаре до рестарта (#30895, #36363); Desktop-чаты нашему процессу не принадлежат | меняет объём задачи: это свой Codex-клиент, а не интеграция Desktop |

Классификация механизмов:

- **Официальный контракт:** deep links; `thread/list`, `thread/read`, `model/list`, `ThreadStatus`, `turn/completed` (для своего процесса); `item/tool/requestUserInput` и `thread/settings/update` (экспериментальные).
- **Публично описанный частный механизм:** `state_5.sqlite`, `codex-dev.db.local_thread_catalog`, rollout JSONL, `.codex-global-state.json`, лог `thread_stream_view_activity_changed`, `models_cache.json`, env `CODEX_APP_SERVER_USE_LOCAL_DAEMON`, Remote Control relay, протокол Codex Micro (HID/BLE).
- **Эвристика:** окно активности по времени, «непрочитан, если завершён позже моего открытия», NLP по тексту.
- **Сигнала нет:** текущий видимый чат и живой статус чатов Desktop через официальный API; внешний ответ на вопрос в чате Desktop.

## Что можно, что требует изменения условий

**Без внутренностей Codex** (сейчас): кнопка/крутилка открывает конкретный чат Codex через `open -b com.openai.codex codex://threads/<id>`; каталог чатов с заголовком, папкой, родителем, архивом и сохранёнными model/effort через `thread/list` своего app-server в read-only режиме (без `thread/resume`).
При этом на тайле Codex-чаты будут без живого состояния: честно показывать «неизвестно», а не «завершён».

**Требует изменения условий** — по каждому пункту отдельное решение пользователя:

1. Живые статусы, ожидание ответа, непрочитанность, восстановление после рестарта — разрешить read-only чтение частных файлов `~/.codex` (rollout JSONL, `.codex-global-state.json`) с явной пометкой «частный формат, может сломаться». Так делают все аналоги.
2. Текущий чат — разрешить чтение лога Desktop (`thread_stream_view_activity_changed`).
3. Model/effort открытого чата — официального пути нет. Варианты: UI-автоматизация пикера (как для Claude Desktop; сейчас запрещено и Computer Use отказал) или менять только defaults для новых чатов через `config.toml` (глобальный побочный эффект, S3).
4. Ответы на вопросы — официального пути нет. Варианты: UI-автоматизация карточки (запрещено), Remote Control relay (облако, недокументировано, баг #53489) или путь C.

**Конкретный блокер** для пунктов 3–4 и живых статусов: Codex Desktop не публикует endpoint своего app-server, а протокол отдаёт живые события и запросы только подписчикам процесса-владельца потока.

## Минимальный ручной эксперимент

Цель — подтвердить блокер на 26.1103, а не обойти его. Выполняет пользователь, read-only, без ответов в чатах:

```bash
ps -Ao pid,ppid,args | grep -i 'codex.*app-server' | grep -v grep
```

```bash
ls -la ~/.codex/app-server-control/ 2>&1
```

**Результат 2026-11-06, проверено:** дочерний процесс `ChatGPT` (pid 47746): `…/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex -c features.code_mode_host=true app-server --analytics-default-enabled -c plugins.codex-app-tools@openai-bundled.mcp_servers.codex_app.enabled=true`, `--listen` нет (stdio по умолчанию); `~/.codex/app-server-control/` нет; вшитый CLI `0.167.0-alpha.3.2`. Путь A на 26.1103 закрыт, как в #41014. Второй `app-server --listen stdio://` принадлежит `cua_node/bin/node_repl`, это не Desktop.

- Если у процесса `app-server` нет `--listen unix://`, есть `-c mcp_servers.codex_app=…`, а сокета нет — путь A на 26.1103 закрыт так же, как в #41014 и #58495. Задачи 2–4 сужаются до пути B или переходят в C.
- Если сокет есть и Desktop слушает его — нужен второй эксперимент: на отдельном тестовом чате клиент подключается через `codex app-server proxy`, делает `initialize` и `thread/read`. Без `thread/resume` и без ответов: проверяется только то, что подключение к процессу Desktop возможно. Запускать только с согласия пользователя.

## Источники

- https://learn.chatgpt.com/docs/app-server · https://learn.chatgpt.com/docs/reference/commands · https://learn.chatgpt.com/docs/cli/reference
- https://github.com/openai/codex/tree/657a993cbee87acf52d14b758ce49dbd46d1b8eb/codex-rs/app-server (README 0.153.2), main `ebe94e25`
- Issues (все открыты, ответов OpenAI нет): [#25914](https://github.com/openai/codex/issues/25914) attach к Desktop · [#40134](https://github.com/openai/codex/issues/40134) внешний app-server для Desktop · [#30895](https://github.com/openai/codex/issues/30895), [#36363](https://github.com/openai/codex/issues/36363) сайдбар не видит внешние потоки · [#29605](https://github.com/openai/codex/issues/29605) выбор окна deep link · [#41014](https://github.com/openai/codex/issues/41014) регрессия shared daemon · [#58495](https://github.com/openai/codex/issues/58495) тот же вопрос на 26.1027 · [#36571](https://github.com/openai/codex/issues/36571) нет списка работающих сессий · [#53489](https://github.com/openai/codex/issues/53489) вопрос висит после внешнего ответа · [#47425](https://github.com/openai/codex/issues/47425) `codex_app` при daemon
- Закрыт сотрудником OpenAI: [#37967](https://github.com/openai/codex/issues/37967) — с CLI 0.157 обычный `codex` использует shared daemon; относится только к CLI.
- Аналоги (HEAD на 2026-11-06): [deck-threads](https://github.com/rschwabco/deck-threads) `7beee95` · [CodexDeck](https://github.com/dada2623/CodexDeck) `7cda7a6` (свой daemon — отдельный runtime, README:162; effort через ⌘⌥E) · [threaddeck-for-codex](https://github.com/y5862000/threaddeck-for-codex) `8cc04f4` (CDP и SIGUSR1-inspector — инъекция) · [stream-deck-codex-micro](https://github.com/DEXCERRONE/stream-deck-codex-micro) `5b69198` (Loupedeck-клиент, правит `keybindings.json`) · [streamdeckcodex](https://github.com/twidtwid/streamdeckcodex) `3549065` (AX-пикер с read-back) · [codex-stream-deck (ckundel2008)](https://github.com/ckundel2008/codex-stream-deck) `8167b7e` (read-only каталог, консервативный unread) · [codex-stream-deck (dazer1234)](https://github.com/dazer1234/codex-stream-deck) `6d7d14b` (Windows, CDP).

## Локальная разведка 2026-11-06 (AX + файлы, с разрешения пользователя)

Чтение только схем и служебных полей, без текста переписки. Desktop 26.1103.31037, вшитый CLI `0.167.0-alpha.3.2`.

### Файлы `~/.codex` (проверено, частный формат)

| Нужно | Источник | Наблюдение |
|---|---|---|
| список, заголовок, cwd, архив, model, effort | `state_5.sqlite` `threads`: `id`, `title`/`name`, `cwd`, `archived`, `model`, `reasoning_effort`, `thread_source` (`user`, `subagent`, `guardian_review`, …), `recency_at_ms`, `is_pinned` | модели в данных: `gpt-6.1-terra`, `gpt-6.1-sol`, effort `low`/`medium` |
| родитель | `thread_spawn_edges (parent_thread_id, child_thread_id, status open/closed)`, `threads.attached_thread_parent_id` | 112 open, 1 closed |
| статус хода | `thread_history_1.sqlite` `thread_turns.status`: `inProgress`, `completed`, `interrupted`, `failed`, `completed_at` | 17 `inProgress` в истории — часть зависших после рестартов; одного статуса мало |
| работает сейчас | `sqlite/codex.db` `active_chat_turns (thread_id, launch_id, observed_at_ms)` | одна строка, совпадает с потоком, у которого последний ход `inProgress`; `launch_id` позволяет отбросить строки прошлых запусков (предположение) |
| непрочитан | `.codex-global-state.json` → `electron-thread-read-state-v1.unreadByIdentity[<account>]["local:<hash>"]` = список id | 1 аккаунт, 1 host, 16 непрочитанных; соответствие синим точкам не сверено. В `state_5.sqlite` есть `thread_read_receipts (first_unread_turn)`, всего 6 строк |
| текущий чат | лог `~/Library/Logs/com.openai.codex/YYYY/MM/DD/codex-desktop-*.log`: `thread_stream_view_activity_changed active=true conversationId=<id> resumeState=resumed streamRole=owner` | есть; полей `rendererWindowFocused` в этой версии нет. AX: заголовок главного окна = заголовок текущего чата (`🎛️ … \| 6 ноя`) |
| смена model/effort | rollout `event_msg` `thread_settings_applied {thread_id, thread_settings{model, …}}` | read-back после переключения |
| вопрос пользователю | rollout `function_call` `request_user_input` | в ноябре ни одного реального вызова, только описание инструмента; формат записи не наблюдался |

### AX (проверено)

- Процесс-потомок Claude доверен для AX; `AXManualAccessibility = true` открывает веб-дерево. Дерево содержит циклы: обход нужен с множеством посещённых (`CFHash`), иначе 300 000 узлов.
- Окно чата: `AXWebArea` с заголовком чата; композер `AXTextArea` `Work with Codex`.
- Пикер: `AXPopUpButton`, `AXTitle` = `GPT-6.1 Sol Medium` / `GPT-6.1 Sol Light` / `GPT-6.1 Terra Medium` — модель и effort читаются без открытия меню. `low` в UI называется `Light` (предположение по совпадению с `reasoning_effort=low` в базе).
- Открытие пикера: `AXPress` и `AXShowMenu` (действия элемента: `AXShowMenu`, `AXScrollToVisible`) возвращают успех, но Radix-меню не открывают ни в фоне, ни на переднем плане. Открывает только `AXFocused = true` на пикере + пробел (keycode 49, `CGEvent.postToPid`), Codex на переднем плане. Закрытие — Esc.
- Меню (`AXGroup` `Select effort`, `radix-…`): `AXMenuItem` `Select model` (подменю моделей не открывалось), `Enable fast mode` (подпись `More usage`), `Reset to default`, `Power` — слайдер effort без значения в AX. Read-back — живая область `AXApplicationStatus`: `GPT-6.1 Terra Standard, 2 of 6.`; подсказка `Use Left and Right arrow keys to adjust power`. У слайдера 6 позиций, а в заголовке пикера в то же время стоит `GPT-6.1 Terra Medium`: соответствие позиций и `low`/`medium`/`high` не установлено, выбор не пробовался.
- Слайдер `Power` (проверено 2026-11-06 на тестовом чате, фокус на `AXMenuItem` `Power`, стрелки ←/→ keycode 123/124): 6 позиций, read-back по `AXApplicationStatus` и по rollout `thread_settings_applied.thread_settings.reasoning_effort`:

  | Позиция | Слайдер | `reasoning_effort` | Заголовок пикера |
  |---|---|---|---|
  | 1 | Light | `low` | `… Light` |
  | 2 | Standard | `medium` | `… Medium` |
  | 3 | Extended | `high` | не снимался |
  | 4 | Extra High | `xhigh` | не снимался |
  | 5 | Max | `max` | не снимался |
  | 6 | Ultra | `ultra` | не снимался |

  Каждый шаг сразу применяется к чату: на каждый шаг пишется `thread_settings_applied`, а `threads.reasoning_effort` в `state_5.sqlite` хранит последнее значение. `config.toml` при этом не меняется (mtime остался до пробы), то есть defaults новых чатов слайдер не трогает. На краях позиция не меняется. Значение возвращено на `Standard`. Как смена ведёт себя во время активного хода, не проверялось.
- Подменю `Select model` открывается через `AXPress` на пункте (rc 0). Пункты — `AXMenuItem`, у выбранного `AXValue = 1` и `MenuItemMarkChar = ✓`: `Default Recommended set of models`, `GPT-6.1 Sol`, `GPT-6 Astra`, `GPT-6.1 Terra` ✓, `GPT-6 Sol`, `GPT-6 Luna`, `GPT-5.6 Sol`, `GPT-5.6 Terra`, `GPT-5.6 Luna`. Обе нужные модели доступны. Выбор модели не пробовался.
- AX-обход: дедупликация только по `CFHash` теряет поддеревья (коллизии хэшей) — отсюда «пустые» чтения ленты и композера. Нужно сравнивать через `CFEqual` внутри одного хэша; после этого пикер и карточка находятся стабильно.
- Статус работы в AX: кнопка `Working for 13m 42s` / `Worked for 8s` в ленте, уведомление `<чат>. Running. Ran command. Open notification`.
- Карточка вопроса (`request_user_input_async`, проверено на тестовом чате «Спроси про цвет»): `AXButton` `Dismiss`, `AXRadioGroup` с `AXRadioButton` по вариантам (`Title` = текст варианта), `AXTextArea` `Or write your own response`, `AXButton` `Dictate`, `Skip`, `Send` (`Enabled=0`, пока ничего не выбрано). Текст вопроса — `AXStaticText`. Несколько вопросов и переходы между ними не наблюдались.
- Файлы при вопросе: в rollout `function_call` `request_user_input_async` с аргументами `{questions:[{title, options:[str]}]}`, ответ инструмента сразу `{"accepted":true}`, затем `task_complete`; ход в `thread_turns` — `completed`, в `active_chat_turns` строки нет. Висящий вопрос в файлах от завершённого хода не отличается: признак «ждёт ответа» — асинхронный вызов в последнем ходе без последующего `userMessage` (эвристика) или карточка в AX у чата в фокусе.
