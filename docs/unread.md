# Unread chats (tile `2/2`)

> Superseded on the device by the `chats` module (`chats.go`, `tg.go`, `vkws.go`): Telegram via MTProto (`gotd/td`, session from Keychain service `tg-digest`; dialogs are refetched on a pushed update and also every 30 s, because the push stream stalls silently after network flaps — verified 12 ноя: unread chats from 07:24 and 07:58 stayed off the tile, the API returned them; each call has a 30 s timeout), VK WorkSpace via the WIM long-poll on the `vkworkspace` MCP session. One list: normal chats first, then muted ones (dimmed to 40% on the tile, full opacity while browsing). Muted: Telegram dialogs muted (`mute_until` in the future) or archived (`folder_id 1`); VK WorkSpace group chats (`…@chat.agent`) with buddylist `mute` ≠ 0 or missing from the buddylist (an archived group drops out of it together with its mute flag); 1:1 chats by `mute` of `rapi/getUserInfo`, fetched with each `getHistory` (the buddylist holds no 1:1 chats, verified 9 ноя: JiraBot `1000000569` and a muted contact give `mute: true`, an unmuted one no field). The startup scan covers the buddylist plus every dialog once seen via `histDlgState`, kept one sn per line in `~/.config/zored-stream/vkws-dialogs`: without it a 1:1 chat unread at daemon start stays hidden until its next message. The Dock-badge code below stays in the tree but is not wired.
>
> VK WorkSpace text: the last message's `text`, else its `parts` joined without `mediaType: quote` (a reply carries the parent as a `quote` part before its own `text` part; verified on `getHistory`). Group chats: `chat.sender` of the last incoming message is the sender; its avatar (same `files/avatar/get`, cached per sn, fetched once) sits as a small disc bottom-left of the chat avatar in the grid and the browse card. 1:1 and outgoing: no overlay.
>
> Messages while browsing: each chat carries its unread incoming messages, oldest first, at most 50. Since 2026-11-19 the browse lives on the three left knobs, top to bottom: крутилка слева, ряд 1 scrolls the shown message (and its photos), ряд 2 walks the messages, ряд 3 the chats; each has a chevron; any of the three presses opens the chat, only ряд 3 (and the tile) holds to mark read; the right knobs are free. Telegram: `messages.getHistory` (limit = unread count) right after each dialogs refresh, only when the dialog's top message or `read_inbox_max_id` changed, muted chats at most once a minute; messages with `id > read_inbox_max_id` and not `out`; a dialog only marked unread gives its top message. Group senders: `from_id` user photo via the same downloader, cached by photo id. VK WorkSpace: the same `getHistory` tail, messages with `msgId > yours.lastRead`, incoming, not `class: event`, sorted by `msgId`; sender avatar by `chat.sender` in groups. Nothing new polls: idle CPU stays at ~3 % (measured 11 ноя, `top`, 3.4 %).
>
> Mark read (hold 400 ms while browsing): Telegram `messages.readHistory` (`channels.readHistory` for channels and supergroups) up to the top message, plus `messages.markDialogUnread` when the dialog was only marked unread. VK WorkSpace `rapi/setDlgState {sn, lastRead: <last msgId>}`, `status.code 20000` = ok (method name and params from the VK WorkSpace client binary). The chat drops from the list at once and stays hidden until its source drops it or a newer message comes (both messengers verified on the device by the user, 11 ноя); a failed call logs `chats: mark … read: …` and brings it back.
>
> Strip: only the message text, white, JetBrains Mono 12, wrapped by characters; крутилка слева, ряд 1 moves the target half a line per detent (clamped to the text height, measured by `cal_text_lines`), the offset eases at 60 Hz only while it moves (P1, P2).
>
> Loop (since 2026-11-23): all three knobs go through the chats in a loop both ways. ряд 3 wraps the chats; ряд 2 past the last message lands on the next chat's first, before the first on the previous chat's last (one chat: wraps its own messages). ряд 1 at an end of the text and photos counts detents (`chatsPushTurns` = 3): the first two ease a white 2 px bar along the strip's bottom (forward) or top (back) edge to 1/3 and 2/3, the third moves on as ряд 2 does, back landing at the end of the text; a turn the other way or 1.5 s idle drops the bar. Detents during the 150 ms chat slide are swallowed. Nothing unread: a gray closed eye (`icons/eye-closed.svg`).
>
> Planned: unread mail in the same list (see [Mail](#mail-planned)).
>
> Photos (`photo.go`, fork `src/photo.ts`): a message carries its pictures. Telegram: `MessageMediaPhoto`, the smallest `PhotoSize` / `PhotoSizeProgressive` with a long side ≥ 700 px (else the largest), downloaded by `InputPhotoFileLocation` from the photo's DC; an album (same `grouped_id`) is one message with the caption of the photo that has one. VK WorkSpace: `filesharing[]` of `getHistory` with `mime image/*` (verified 19 ноя: png, jpeg, webp in the tail; the message `text` is the file link); `GET <base_url>/api/v151/files/info/<id>/?aimsid&previews=800` returns `result.previews["800"]`, a link that downloads without more auth (verified: 800 px png of a 4 MB original, 1 MB). While a message is shown in browse its photos load in the background (40 kept, a failure retried after 30 s, logged `photo <key>: …`). крутилка слева, ряд 1 past the end of the text walks them; the shown one goes to the fork over `:47130 GET /photo` (long poll) and `GET /photo/image?id&w&h` (raw RGB cropped to fill); the fork turns and cuts it over the 11 tiles around this one and eats a touch on them (`/photo/close`). Off outside browse and in a call. Decoding: Go `image/jpeg`, `image/png`, `x/image/webp`; anything else fails loudly with `415`.

Module `unread` of `zored_stream`, port 47130 (`/tile/22/down`, `/tile/22/up`). Shares the mic tile through the `tile_22` slot (F23): a live call wins, otherwise the tile shows unread apps. Крутилка слева, ряд 3: `/knob/down`, `/knob/up` = the tile's tap, `/knob/forward`, `/knob/back` browse (a turn outside browse starts it).

## Source

- Dock badges: every Dock item's `AXStatusLabel` (`com.apple.dock` → list → items), matched by the Dock title (`Telegram`, `VK WorkSpace Beta`). Non-numeric badges count as 1. Verified for Telegram.
- Age: the daemon stamps the moment a badge grows. Badges already present at daemon start have no age (empty). Neither the Dock nor the banners carry a send time.
- Poll every 2 s (P2): one small AX tree, also ticks the age.

## Tile

- `unread_svg`: one circle per app with unread, Telegram `#2AABEE`, VK WorkSpace `#1A3A8F`; white count centered, black outline, capped `99+`; gray age below: `Nm` (`59+m`), `Nh` (`23+h`), `Nd` (`9+d`).
- Tap: opens the next app with unread (`open -b <bundle>`), round robin.
- Nothing unread: a small white eye.

## Dead ends

- Notification banners (`com.apple.notificationcenterui`, subrole `AXNotificationCenterBanner`): title and body only, no app and no time (verified); attributing them by badge timing is fragile.
- Notification Center database (`group.com.apple.usernoted/db2`): TCC-protected, `Operation not permitted` without Full Disk Access.

## Mail (planned)

- Source: VK WorkSpace mail, unread threads of Inbox (the `vkworkspace` MCP exposes `mail-unread`, `mail-read-thread`, `mail-mark-message`; the HTTP API behind it and whether the daemon's WIM session can call it: not checked).
- Entry: one per thread, app `mail`, title = sender name, text = subject + snippet, messages = unread letters of the thread, oldest first; muted = newsletters (`List-Unsubscribe`) or not Inbox.
- Sender icon, first hit wins: VK WorkSpace avatar of a colleague by email (`files/avatar/get`, as chat senders; assumed to accept an email), BIMI logo of the sender domain, the domain favicon, initials on dark gray like a chat without avatar.
- Open: the thread in the web mail; hold = mark the thread read.
- Check first: one `mail-unread` call for latency and fields, then whether a colleague avatar resolves by email.
