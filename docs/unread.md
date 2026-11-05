# Unread chats (tile `2/2`)

> Superseded on the device by the `chats` module (`chats.go`, `tg.go`, `vkws.go`): Telegram via MTProto (`gotd/td`, session from Keychain service `tg-digest`), VK WorkSpace via the WIM long-poll on the `vkworkspace` MCP session. One list: normal chats first, then muted ones (dimmed to 40% on the tile, full opacity while browsing). Muted: Telegram dialogs muted (`mute_until` in the future) or archived (`folder_id 1`); VK WorkSpace chats with buddylist `mute` ≠ 0 or missing from the buddylist — an archived (hidden) chat, e.g. JiraBot `1000000569`, drops out of the buddylist together with its mute flag and reaches us only via `histDlgState`. The Dock-badge code below stays in the tree but is not wired.
>
> VK WorkSpace text: the last message's `text`, else its `parts` joined without `mediaType: quote` (a reply carries the parent as a `quote` part before its own `text` part; verified on `getHistory`). Group chats: `chat.sender` of the last incoming message is the sender; its avatar (same `files/avatar/get`, cached per sn, fetched once) sits as a small disc bottom-left of the chat avatar in the grid and the browse card. 1:1 and outgoing: no overlay.

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
