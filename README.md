# stream

Notes, configs and tooling for stream controllers (macro decks).
First device: **Razer Stream Controller** — a rebranded Loupedeck Live.

## Hardware

| | Razer Stream Controller |
|---|---|
| Controls | 12 touch tiles, 6 rotary encoders (turn + press), 8 RGB buttons |
| Same hardware as | Loupedeck Live |
| USB | VID `0x1532`, PID `0x0D06` (Stream Controller X: `0x0D09`) |
| Transport | USB CDC serial → WebSocket upgrade → binary frames |
| Protocol notes | [ShoelessTim/RazerStream PROTOCOL.md](https://github.com/ShoelessTim/RazerStream/blob/main/PROTOCOL.md) |

## Ways to drive it

| Approach | macOS | Windows | Notes |
|---|---|---|---|
| [Loupedeck software](https://loupedeck.com/downloads/) | ✅ | ✅ | Official. Razer support kept in 6.4.1+ |
| [ShoelessTim/RazerStream](https://github.com/ShoelessTim/RazerStream) | ✅ | ❌ | Native Swift host app, talks the wire protocol directly |
| [@loupedeck/node](https://github.com/Julusian/node-loupedeck) | ✅ | ✅ | Node library, base for a custom host |
| [foxxyz/loupedeck](https://github.com/foxxyz/loupedeck) | ✅ | ✅ | Node API: buttons, knobs, touch, drawing |
| [Bitfocus Companion](https://github.com/bitfocus/companion) | ✅ | ✅ | Open-source control surface app, supports Loupedeck Live / RSC |

None of these replace the device firmware: they are host-side drivers.
No public custom firmware for this device is known yet.

## Status

See [PLAN.md](PLAN.md).

Not affiliated with Razer, Loupedeck or Logitech.
