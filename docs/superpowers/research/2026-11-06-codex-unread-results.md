# Непрочитанность чатов Codex Desktop: результаты проверки

План: `2026-11-06-codex-unread-check.md`. Дата: 2026-11-06…07.

## Версия и метод

- Codex Desktop `26.1103.31037` (`CFBundleShortVersionString`).
- UI-действия выполнял человек, агент только читал файлы: `~/.codex/.codex-global-state.json` (JSON целиком) и `~/.codex/state_5.sqlite` (`mode=ro`, WAL учтён SQLite). App-server не запускался, Computer Use не использовался.
- ID чатов, identity и bucket в журнале приведены как первые 8 символов sha256. Чат ищется по `thread_name` в `~/.codex/session_index.jsonl`.
- Тестовые чаты: A `4ba12353` («Спроектировать дайджест AI-коммитов»), B `16a7ab28` («Запусти цикл в терминале»).

## Структура (наблюдено)

```
electron-thread-read-state-v1
  version: 1
  unreadByIdentity: { <identity>: { "local:<64hex>": [threadId, ...] } }
  legacyMigration: { identityKey: <identity>, unreadThreadIdsByHostId: {local}, adoptedHostIds: {local} }
```

- `<identity>` — 64 hex (похоже на sha256). Совпадает с `legacyMigration.identityKey`. На этой машине одна identity и один bucket с префиксом `local:`.
- `thread_read_receipts(thread_id, first_unread_turn, revision, last_published_turn)` в `state_5.sqlite`: непрочитанный ход отмечен так: `first_unread_turn` не NULL и равен `last_published_turn`.

## Наблюдения

| Время | Чат | Действие | UI | global-state (n в bucket, чат в нём) | receipts first_unread | Задержка |
|---|---|---|---|---|---|---|
| 23:09 | A | исходно: непрочитан | точка | 16, да | = last_pub | — |
| 23:10 | A | открыл | точки нет | 15, нет | NULL, revision сменилась | ≤1 мин |
| 23:41 | A | Mark as unread, затем начат ход | точка | 16, да | **NULL** (сменились только last_pub и revision) | ≤1 мин |
| 23:43 | A | ход закончился во время просмотра, прочитал | точки нет | 15, нет | NULL | ≤1 мин |
| 23:43 | A | архив | — | 15, нет | NULL; `threads.archived=1` | ≤1 мин |
| 23:44 | A | восстановил | точки нет | 15, нет | NULL; `archived=0` | ≤1 мин |
| 00:00 | B | новый чат, ход закончился в фоне | точка | 16, да | = last_pub | ≤1 мин |
| 00:01 | B | открыл | точки нет | 15, нет | NULL | ≤1 мин |

Задержки — верхняя граница: снимок снимался после сообщения человека. Точнее не мерили.

## Вывод

Проверено: точку в UI повторяет `electron-thread-read-state-v1.unreadByIdentity`. Receipts в sqlite ручную пометку «непрочитанным» **не видят**, поэтому как источник правды не годятся. Для подстраховки подходят только на автоматической непрочитанности.

Алгоритм адаптера (получен на одной identity, дальше гипотеза):

1. Прочитать и распарсить `.codex-global-state.json`. Ошибка чтения или разбора → `unknown`.
2. Нет `electron-thread-read-state-v1` или `version != 1` → `unknown`.
3. identity = `legacyMigration.identityKey`. Пусто, или нет ключа в `unreadByIdentity` → `unknown`.
4. Bucket = единственный ключ с префиксом `local:`. Ни одного или несколько → `unknown`.
5. Значение bucket не список → `unknown`. Чат в списке → `unread`, иначе `read`.
6. Чата нет в `threads` (sqlite открыт read-only) → `unknown`, чтобы чужой или удалённый ID не дал `read`.

## Не проверено

- Перезапуск приложения, несколько окон, несколько аккаунтов или identity, удалённые хосты (не `local:`). В частности, не подтверждено, что `legacyMigration.identityKey` всегда указывает на текущую identity. При смене аккаунта значение может устареть.
- Точная задержка записи на диск и атомарность записи JSON: читатель может поймать полузаписанный файл. Тогда нужен повтор, а не `read`.
- Восстановление из архива проверено на одном чате. Что происходит с непрочитанным чатом при архивации, не проверяли.
- Ветки `unknown` проверены рассуждением по структуре, а не на тестовых данных.
