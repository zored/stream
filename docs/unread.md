# Unread chats (tile `2/2`)

Module `unread` of `zored_stream`, port 47130 (`/tile/22/down`, `/tile/22/up`). Shares the mic tile through the `tile_22` slot (F23): a live call wins, otherwise the tile shows unread chats.

## Sources (Accessibility only)

- **Dock badges**: every Dock item's `AXStatusLabel` (`com.apple.dock` → list → items). The number is the app's unread count, non-numeric badges count as 1. Verified for Telegram.
- **Notification banners**: `com.apple.notificationcenterui` window, groups with subrole `AXNotificationCenterBanner` / `AXNotificationCenterAlert`, child `AXIdentifier=title` = chat name, group `AXIdentifier` = a per-banner UUID. Verified on macOS 26 with `display notification`.
- A banner carries no app name or bundle id (verified). It is attributed to the app whose Dock badge rose within the last 5 s; otherwise it is dropped.
- A badge back at 0 clears that app's chats. An app with a badge but no seen banner shows its letter (`t`, `w`).

Poll every 2 s (P2): banners stay ~5 s, one scan reads two small AX trees. Dead end: the Notification Center database (`group.com.apple.usernoted/db2`) is TCC-protected, `Operation not permitted` without Full Disk Access.

## Tile

- `unread_svg`: first letter of the chat (white), the app's count (gray, top right), dots when there is more than one chat.
- Tap: next chat. Hold ≥ 300 ms: `open -b <bundle>`. No deep link into a chat.
- Apps: Telegram `ru.keepcoder.Telegram`, VK WorkSpace Beta `ru.mail.messenger-biz-avocado-desktop.beta` (matched by the Dock title).

## Limits

- Muted chats, notifications off or Do Not Disturb: no banner, only the count and the app letter.
- Two apps raising badges within 5 s of each other may swap a banner.
