# Unread chats (tile `2/2`)

> Superseded on the device by the `chats` module (`chats.go`, `tg.go`, `vkws.go`): Telegram via MTProto (`gotd/td`, session from Keychain service `tg-digest`; dialogs are refetched on a pushed update and also every 30 s, because the push stream stalls silently after network flaps — verified 12 ноя: unread chats from 07:24 and 07:58 stayed off the tile, the API returned them; each call has a 30 s timeout), VK WorkSpace via the WIM long-poll on the `vkworkspace` MCP session. One list: normal chats first, then muted ones (dimmed to 40% on the tile, full opacity while browsing). Muted: Telegram dialogs muted (`mute_until` in the future) or archived (`folder_id 1`); VK WorkSpace group chats (`…@chat.agent`) with buddylist `mute` ≠ 0 or missing from the buddylist (an archived group drops out of it together with its mute flag); 1:1 chats by `mute` of `rapi/getUserInfo`, fetched with each `getHistory` (the buddylist holds no 1:1 chats, verified 9 ноя: JiraBot `1000000569` and a muted contact give `mute: true`, an unmuted one no field). The startup scan covers the buddylist plus every dialog once seen via `histDlgState`, kept one sn per line in `~/.config/zored-stream/vkws-dialogs`: without it a 1:1 chat unread at daemon start stays hidden until its next message. The Dock-badge code below stays in the tree but is not wired.
>
> VK WorkSpace text: the last message's `text`, else its `parts` joined without `mediaType: quote` (a reply carries the parent as a `quote` part before its own `text` part; verified on `getHistory`). Group chats: `chat.sender` of the last incoming message is the sender; its avatar (same `files/avatar/get`, cached per sn, fetched once) sits as a small disc bottom-left of the chat avatar in the grid and the browse card. 1:1 and outgoing: no overlay.
>
> Messages while browsing: each chat carries its unread incoming messages, oldest first, at most 50. Telegram: `messages.getHistory` (limit = unread count) right after each dialogs refresh, only when the dialog's top message or `read_inbox_max_id` changed, muted chats at most once a minute; messages with `id > read_inbox_max_id` and not `out`; a dialog only marked unread gives its top message. Group senders: `from_id` user photo via the same downloader, cached by photo id. VK WorkSpace: the same `getHistory` tail, messages with `msgId > yours.lastRead`, incoming, not `class: event`, sorted by `msgId`; sender avatar by `chat.sender` in groups. Nothing new polls: idle CPU stays at ~3 % (measured 11 ноя, `top`, 3.4 %).
>
> Mark read (hold 400 ms while browsing): Telegram `messages.readHistory` (`channels.readHistory` for channels and supergroups) up to the top message, plus `messages.markDialogUnread` when the dialog was only marked unread. VK WorkSpace `rapi/setDlgState {sn, lastRead: <last msgId>}`, `status.code 20000` = ok (method name and params from the VK WorkSpace client binary). The chat drops from the list at once and stays hidden until its source drops it or a newer message comes (both messengers verified on the device by the user, 11 ноя); a failed call logs `chats: mark … read: …` and brings it back.
>
> Strip: only the message text, white, JetBrains Mono 12, wrapped by characters; крутилка справа, ряд 3 moves the target half a line per detent (clamped to the text height, measured by `cal_text_lines`), the offset eases at 60 Hz only while it moves (P1, P2).

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
