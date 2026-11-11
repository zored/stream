# Weather

Module `weather` of `zored_stream`, port 47131 (`POST /tile/23/press`), file `weather.go`.

- Source: [Open-Meteo](https://open-meteo.com/) forecast API, free, no key. Direct access timed out on 2026-11-12 (the tile went gray), so the daemon goes through the `-usage-proxy` HTTP proxy (verified: 0.7 s).
  `current` + `hourly` `temperature_2m`, `apparent_temperature`, `weather_code`, `is_day`, 3 days, `timezone=auto`.
  Rejected: Yandex Weather (key, paid tiers), wttr.in (often down), MET Norway (fewer condition codes).
- Location: flags `-weather-lat` / `-weather-lon`, default Moscow. CoreLocation would need a permission grant.
- Poll every 20 min; on error retry after 1 min, doubling up to 20 min. Data older than 90 min turns the tile gray.
- Tile `2/3` (тайл ряд 3, колонка 2) through the `tile_23` slot (F23): a live call wins (camera), otherwise weather.
  Variables: `tile_23`, `weather_svg`.
- Look: Bootstrap icon by WMO code (`sun`/`moon`, `cloud-sun`/`cloud-moon`, `clouds`, `cloud-fog2`, `cloud-drizzle`,
  `cloud-rain`, `cloud-rain-heavy`, `cloud-sleet`, `cloud-snow`, `cloud-lightning-rain`), caption `<temp> <feels like>` rounded (temperature white, feels-like gray, no separator), no units, layout as the keep-awake tile.
- Tap: 5 s browse. Крутилка слева, ряд 3 walks the slots: now, then утро 08, день 13, вечер 18, ночь 23 of each day
  after now; the slot shows white on top as `пт 6 утро`; now carries the label too, and outside a browse no label is drawn (`чт 12 день`: утро 5–11, день 11–17, вечер 17–23, ночь). Each turn extends the window; a second tap, a knob press or
  5 s idle returns to now. While chats browse, the knob stays with chats.
- Companion's SVG renderer ignores `paint-order`: a stroked label drew black over its fill, so labels have no stroke.
