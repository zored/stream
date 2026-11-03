import base64
import json
import os
import random
import string
import sys

PROFILE = sys.argv[2] if len(sys.argv) > 2 else "mac"
if PROFILE not in ("mac", "windows"):
    sys.exit(f"unknown profile {PROFILE!r}, want mac or windows")
WINDOWS = PROFILE == "windows"
ALPHABET = string.ascii_letters + string.digits + "_-"
RAINBOW = [0xFF0000, 0xFF8000, 0xFFFF00, 0x00FF00, 0x00FFFF, 0x0000FF, 0x8000FF, 0xFF00FF]
STREAMD = "http://127.0.0.1:47113"
VKCALL = "http://127.0.0.1:47114"
YMUSIC = "http://127.0.0.1:47115"
AI = "http://127.0.0.1:47117"
BRIGHTNESS = "http://127.0.0.1:47118"
SEEK = "http://127.0.0.1:47119"
SCROLL = "http://127.0.0.1:47121"
AGENTS = "http://127.0.0.1:47120"
POWER = "http://127.0.0.1:47122"
HEADPHONES = "http://127.0.0.1:47123"
WIFI = "http://127.0.0.1:47124"
KVM = "http://127.0.0.1:47125"
CALENDAR = "http://127.0.0.1:47126"
INPUT = "http://127.0.0.1:47127"
EMOJI = "http://127.0.0.1:47128"
TIMERS = "http://127.0.0.1:47129"
HOLD_MS = 400
ICONS = os.path.join(os.path.dirname(os.path.abspath(__file__)), "icons")
CUSTOM_VARIABLES = {name: "" for name in (
    "volume", "bar_value", "bar_alpha", "bar_mode", "ctl_brightness", "np_playing", "np_title", "np_artist", "np_track", "np_svg", "np_app", "np_art", "np_glyph", "np_progress", "np_tint", "np_skip", "np_skip_x", "ym_like", "call_mic", "call_cam",
    "ai_strip_svg", "ai_session", "ai_session_reset", "ai_day", "ai_week", "ai_week_reset", "ai_level",
    "ai_model", "ai_effort", "ai_state", "agents_led", "agents_svg", "dictation", "choice_on", "power_svg", "headphones_svg", "wifi_svg", "kvm_led", "cal_card", "cal_browse", "cal_icon", "tile_24", "timer_svg", "timer_ring", "input_svg", "emoji_on", "emoji_l", "emoji_r", "emoji_tile", *(f"choice_{b}" for b in range(1, 9)))
    if not (WINDOWS and name in ("call_cam", "ym_like", "dictation", "power_svg", "headphones_svg", "wifi_svg", "kvm_led", "cal_card", "cal_browse", "cal_icon", "tile_24", "timer_svg", "timer_ring", "input_svg", "emoji_on", "emoji_l", "emoji_r", "emoji_tile"))}
BAR_ALPHA = "$(custom:bar_alpha)"
PLAYING = "$(custom:np_playing) == 1"
NO_ART = '$(custom:np_art) == ""'
rng = random.Random()


def nid():
    return "".join(rng.choice(ALPHABET) for _ in range(21))


def v(value):
    return {"value": value, "isExpression": False}


def expr(value):
    return {"value": value, "isExpression": True}


def prop(value):
    return value if isinstance(value, dict) else v(value)


def geometry(enabled=None, opacity=None, x=None, y=None, width=None, height=None, rotation=None):
    return {"enabled": enabled or v(True), "opacity": opacity or v(100), "x": x or v(0), "y": y or v(0),
            "width": width or v(100), "height": height or v(100), "rotation": rotation or v(0)}


def canvas(decoration="none"):
    return {"id": "canvas", "name": "Canvas", "usage": "auto", "type": "canvas",
            "decoration": v(decoration), "showStatusIcons": v("none")}


def box(layer_id, color, **geo):
    return {"id": layer_id, "name": layer_id, "usage": "auto", "type": "box", "pinnedProperties": ["color"],
            **geometry(**geo), "color": prop(color), "cornerRadius": v(0), "borderWidth": v(0), "borderColor": v(0),
            "borderPosition": v("inside")}


def text(layer_id, value, color=0xFFFFFF, fontsize=100, shrink=True, **geo):
    return {"id": layer_id, "name": layer_id, "usage": "auto", "type": "text",
            "pinnedProperties": ["text", "fontsize", "fontsizeAllowShrink", "color", "halign", "valign"],
            **geometry(**geo), "text": prop(value), "color": prop(color), "halign": v("center"), "valign": v("center"),
            "fontsize": v(fontsize), "fontsizeAllowShrink": v(shrink), "font": v("companion-sans"), "weight": v("normal"),
            "styles": v([]), "outlineColor": v(4278190080)}


def image(layer_id, data, fill="fit", **geo):
    return {"id": layer_id, "name": layer_id, "usage": "auto", "type": "image",
            "pinnedProperties": ["base64Image", "halign", "valign"], **geometry(**geo), "base64Image": prop(data),
            "halign": v("center"), "valign": v("center"), "fillMode": v(fill)}


def icon(name):
    with open(os.path.join(ICONS, f"{name}.svg"), "rb") as f:
        return "data:image/svg+xml;base64," + base64.b64encode(f.read()).decode()


def labelled(label, bg=0x000000, fg=0xFFFFFF):
    return [canvas(), box("box0", bg), text("text0", label, fg)]


def action(definition, options):
    return {"type": "action", "id": nid(), "connectionId": "internal", "definitionId": definition,
            "options": {k: x if isinstance(x, dict) else v(x) for k, x in options.items()}, "upgradeIndex": None}


def log(message):
    return action("custom_log", {"message": message})


def sh(command, target=None):
    return action("exec", {"path": command, "cwd": "", "timeout": 2000, "targetVariable": target})


def mediakey(key):
    return post(f"{STREAMD}/key/{key}")


def post(url):
    return sh(f"curl -fsS -m 1 -X POST {url}")


def group(actions):
    return {"type": "action", "id": nid(), "connectionId": "internal", "definitionId": "action_group",
            "options": {"execution_mode": v("sequential")}, "children": {"default": actions}, "upgradeIndex": None}


def brightness(path):
    return [group([sh(f"curl -fsS -m 1 -X POST {BRIGHTNESS}{path}", "ctl_brightness"),
                   action("set_brightness", {"surfaceId": "self", "brightness": expr("$(custom:ctl_brightness)")})])]


def streamd(path):
    return [post(STREAMD + path)]


def vkcall(path):
    return [post(VKCALL + path)]


def ymusic(path):
    return None if WINDOWS else [post(YMUSIC + path)]


def ai(path):
    return [post(f'"{AI}{path}"' if WINDOWS else f"'{AI}{path}'")]


def button(style, down=(), up=(), left=None, right=None, held=None):
    sets = {"down": list(down), "up": list(up)}
    if held:
        sets[str(HOLD_MS)] = list(held)
    rotary = left is not None or right is not None
    if rotary:
        sets["rotate_left"] = list(left or [])
        sets["rotate_right"] = list(right or [])
    return {
        "type": "button-layered",
        "options": {"stepProgression": "auto", "stepExpression": "", "rotaryActions": rotary,
                    "canModifyStyleInApis": True, "notes": ""},
        "style": {"layers": style},
        "feedbacks": [],
        "steps": {"0": {"action_sets": sets, "options": {"runWhileHeld": [HOLD_MS] if held else []}}},
        "localVariables": [],
    }


def logged(name, label, bg, fg=0xFFFFFF, rotary=False):
    events = {"down": [log(f"TEST {name} down")], "up": [log(f"TEST {name} up")]}
    if rotary:
        events["left"] = [log(f"TEST {name} left")]
        events["right"] = [log(f"TEST {name} right")]
    return button(labelled(label, bg, fg), **events)


def strip_image(layer_id, var, row, rotation, opacity=None):
    cell_w, cell_h, strip_h = 60, 90, 270
    center_y = strip_h / 2 - cell_h * row
    return image(layer_id, expr(f"$(custom:{var})"), opacity=opacity, rotation=v(rotation),
                 x=v(round((cell_w / 2 - strip_h / 2) / cell_w * 100, 2)), width=v(round(strip_h / cell_w * 100, 2)),
                 y=v(round((center_y - cell_w / 2) / cell_h * 100, 2)), height=v(round(cell_w / cell_h * 100, 2)))


def cell_fill(var, row):
    base = (2 - row) * 100 / 3
    return f"max(0, min(100, ($(custom:{var}) - {base:.2f}) * 3))"


def volume_cell(row):
    fill = cell_fill("bar_value", row)
    progress = cell_fill("np_progress", row)
    visible = expr(BAR_ALPHA)
    hidden = expr(f"100 - {BAR_ALPHA}")
    color = expr(f'$(custom:bar_mode) == "ctl" ? {0x808080} : {0xFFFFFF}')
    return button([
        canvas(),
        box("progress", expr("$(custom:np_tint)"), opacity=hidden, y=expr(f"100 - {progress}"), height=expr(progress)),
        strip_image("np", "np_svg", row, 90, hidden),
        box("track", 0x000000, opacity=visible),
        box("fill", color, opacity=visible, y=expr(f"100 - {fill}"), height=expr(fill)),
        *(skip_arrows() if row == 2 else ()),
        *emoji_strip("emoji_l", row),
    ])


def skip_arrows():
    return [image(f"skip-{name}", icon(f"skip-{name}"), "fit",
                  opacity=expr(f"$(custom:np_skip) == {direction} ? 100 : 0"),
                  x=expr("$(custom:np_skip_x)"), y=v(44), width=v(88), height=v(50))
            for name, direction in (("forward", 1), ("back", -1))]


def play_pause():
    size = "(80 + $(custom:np_glyph) / 5)"
    return button([
        canvas(),
        box("box0", 0x000000),
        text("icon", expr(f'{PLAYING} ? "⏸" : "▶"'), fontsize=110, opacity=expr(f"{NO_ART} ? 100 : 0")),
        image("cover", expr("$(custom:np_art)"), "crop", x=v(6), y=v(6), width=v(88), height=v(88)),
        *(image(name, icon(f"{name}-glow"), "fill",
                opacity=expr(f"{NO_ART} || ({PLAYING}) != {shown} ? 0 : $(custom:np_glyph)"),
                x=expr(f"(100 - {size}) / 2"), y=expr(f"(100 - {size}) / 2"), width=expr(size), height=expr(size))
          for name, shown in (("play", "false"), ("pause", "true"))),
    ], down=streamd("/nowplaying/show"), up=[mediakey("play")], held=ymusic("/like"))


def by_state(var, colors):
    out = "0"
    for state, color in reversed(colors.items()):
        out = f'$(custom:{var}) == "{state}" ? ({color}) : ({out})'
    return expr(out)


def call_actions(var):
    return {"down": vkcall("/mic/down"), "up": vkcall("/mic/up")} if var == "call_mic" else {"down": vkcall("/cam/down")}


def call_led(var):
    return button([canvas(), box("box0", by_state(var, {
        "on": f"$(internal:time_unix) % 4 == 0 ? {0x003300} : {0x00FF00}",
        "off": 0x400000,
        "pending": 0xFFB000,
        "unknown": f"$(internal:time_unix) % 2 == 0 ? {0xFFB000} : 0",
    }))], **call_actions(var))


def call_tile(var, name):
    state = f"$(custom:{var})"
    return button([
        canvas(),
        box("box0", by_state(var, {"on": 0x1B5E20, "off": 0xB3261E, "pending": 0x8A6100, "unknown": 0x8A6100})),
        image("on", icon(name), opacity=expr(f'{state} == "on" ? 100 : 0'), x=v(20), y=v(20), width=v(60), height=v(60)),
        image("off", icon(f"{name}-off"), opacity=expr(f'{state} == "off" ? 100 : 0'), x=v(20), y=v(20), width=v(60), height=v(60)),
        text("unknown", expr(f'{state} == "pending" ? "…" : "?"'), fontsize=60,
             opacity=expr(f'{state} == "unknown" || {state} == "pending" ? 100 : 0')),
    ], **call_actions(var))


IN_CALL = '$(custom:call_mic) != "none"'


def hangup_led():
    return button([canvas(), box("box0", expr(f"{IN_CALL} ? {0xFFFFFF} : 0"))], down=vkcall("/call/end"))


DIGITS_SVG = ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="6 -74 289 75"><text font-family="JetBrains Mono" '
              'font-size="100" fill="{fill}"', "/text></svg>")


def b64(value):
    return base64.b64encode(value.encode()).decode()


def lookup(table, var):
    return f'substr("{"".join(map(b64, table))}", {var} * 4, {var} * 4 + 4)'


def digits_image(fill, first, second):
    head, tail = DIGITS_SVG
    head = head.format(fill=fill)
    head += " " * (-(len(head) + 1) % 3) + ">"
    return expr(f'concat("data:image/svg+xml;base64,{b64(head)}", {first}, {second}, "{b64(tail)}")')


def time_image():
    return digits_image("#fff", lookup([f"{h:02d}:" for h in range(24)], "$(internal:time_h)"),
                        lookup([f"{m:02d}<" for m in range(60)], "$(internal:time_m)"))


def date_image():
    return digits_image("#808080", lookup([f"{d:02d}." for d in range(32)], "$(internal:date_d)"),
                        lookup([f"{m:02d}<" for m in range(13)], "$(internal:date_m)"))


def clock():
    if WINDOWS:
        return button([canvas(), box("box0", 0x000000),
                       image("time", time_image(), x=v(6), y=v(6), width=v(88), height=v(58)),
                       image("date", date_image(), x=v(6), y=v(74), width=v(88), height=v(17))])
    ring = '$(custom:timer_ring) == 1 && $(internal:time_unix) % 2 == 0'
    return button([canvas(), box("box0", expr(f"{ring} ? {0xFFFFFF} : 0")),
                   image("time", time_image(), opacity=expr(f"{ring} ? 0 : 100"), x=v(6), y=v(6), width=v(88), height=v(58)),
                   image("timer", expr("$(custom:timer_svg)"), x=v(6), y=v(70), width=v(88), height=v(24))],
                  down=[post(TIMERS + "/timer/down")], up=[post(TIMERS + "/timer/up")])


CAL = '$(custom:tile_24) == "cal"'


def day_tile():
    idle = expr(f"{CAL} && $(custom:cal_browse) != 1")
    return button([canvas(), box("box0", 0x000000),
                   image("date", date_image(), enabled=idle, x=v(6), y=v(6), width=v(88), height=v(17)),
                   image("card", expr("$(custom:cal_card)"), enabled=idle, x=v(6), y=v(28), width=v(88), height=v(66)),
                   image("act", expr("$(custom:cal_icon)"), enabled=expr(f"{CAL} && $(custom:cal_browse) == 1"),
                         x=v(6), y=v(6), width=v(88), height=v(88)),
                   image("hangup", icon("hangup"), enabled=expr('$(custom:tile_24) == "call"'),
                         x=v(15), y=v(15), width=v(70), height=v(70))],
                  down=[post(CALENDAR + "/tile/24/press")])


def ai_switch():
    return button(labelled("AI"), down=ai("/ai/down"), up=ai("/ai/up"), left=ai("/ai/turn?d=-1"), right=ai("/ai/turn?d=1"))


def ai_strip(row):
    return button([canvas(), box("box0", 0x000000),
                   image("strip", expr("$(custom:ai_strip_svg)"), "fill", y=v(-100 * row), height=v(300)),
                   *emoji_strip("emoji_r", row)])


def emoji_strip(var, row):
    if WINDOWS:
        return ()
    on = expr("$(custom:emoji_on) == 1")
    return (box("emoji_bg", 0x000000, enabled=on),
            image("emoji", expr(f"$(custom:{var})"), "fill", enabled=on, y=v(-100 * row), height=v(300)))


def emoji_tile():
    return button([canvas(), box("box0", 0x000000),
                   image("emoji", expr("$(custom:emoji_tile)"), x=v(6), y=v(6), width=v(88), height=v(88))],
                  down=[post(EMOJI + "/emoji/tile")])


def agents_led():
    return button([canvas(), box("box0", expr("$(custom:agents_led)"))],
                  up=[post(AGENTS + "/agent/focus")], held=[post(AGENTS + "/agent/new")])


def agents_tile():
    return button([canvas(), box("box0", 0x000000),
                   image("dots", expr("$(custom:agents_svg)"), x=v(6), y=v(6), width=v(88), height=v(88))],
                  up=[post(AGENTS + "/agent/focus")], held=[post(AGENTS + "/agent/new")])


def dictation_led():
    color = by_state("dictation", {"on": f"$(internal:time_unix) % 2 == 0 ? {0x00FF00} : 0", "off": 0xFF0000})
    return button([canvas(), box("box0", color)], down=ai("/dictation/down"), up=ai("/dictation/up"))


def awake_tile():
    return button([canvas(), box("box0", 0x000000),
                   image("sun", expr("$(custom:power_svg)"), x=v(6), y=v(6), width=v(88), height=v(88))],
                  down=[post(POWER + "/power/down")], up=[post(POWER + "/power/up")])


def headphones_tile():
    return button([canvas(), box("box0", 0x000000),
                   image("headphones", expr("$(custom:headphones_svg)"), x=v(6), y=v(6), width=v(88), height=v(88))],
                  down=[post(HEADPHONES + "/headphones/toggle")])


def input_tile():
    return button([canvas(), box("box0", 0x000000),
                   image("layout", expr("$(custom:input_svg)"), x=v(6), y=v(6), width=v(88), height=v(88))],
                  down=[post(INPUT + "/input/next")])


def wifi_tile():
    return button([canvas(), box("box0", 0x000000),
                   image("wifi", expr("$(custom:wifi_svg)"), x=v(6), y=v(6), width=v(88), height=v(88))],
                  up=[post(WIFI + "/wifi/touch")], held=[post(WIFI + "/wifi/power")])


def kvm_led():
    return button([canvas(), box("box0", expr("$(custom:kvm_led)"))], up=[post(KVM + "/kvm/windows")], held=[post(KVM + "/kvm/pip")])


def choice_gated(b, control):
    sets = control["steps"]["0"]["action_sets"]
    for key, edge in (("down", "down"), ("up", "up"), (str(HOLD_MS), "up")):
        if key not in sets:
            continue
        assert all(a["definitionId"] == "exec" for a in sets[key]), (b, key)
        fallback = " ; ".join(a["options"]["path"]["value"] for a in sets[key]) or "true"
        sets[key] = [sh(f"curl -fs -m 1 -X POST '{AGENTS}/choice/{edge}?b={b}' >/dev/null || {{ {fallback} ; }}")]
    background = next(layer for layer in control["style"]["layers"] if layer["id"] == "box0")
    color = background["color"]
    own = color["value"] if color["isExpression"] else str(color["value"])
    background["color"] = expr(f"$(custom:choice_on) == 1 ? $(custom:choice_{b}) : ({own})")
    return control


def test_page():
    grid = {}
    for r in range(3):
        grid[(r, 0)] = logged(f"knob-L{r + 1}", f"KL{r + 1}", 0, rotary=True)
        grid[(r, 7)] = logged(f"knob-R{r + 1}", f"KR{r + 1}", 0, rotary=True)
        grid[(r, 1)] = logged(f"strip-L{r + 1}", f"SL{r + 1}", 0x303030)
        grid[(r, 6)] = logged(f"strip-R{r + 1}", f"SR{r + 1}", 0x303030)
        for c in range(2, 6):
            n = r * 4 + c - 1
            grid[(r, c)] = logged(f"tile-{n}", f"T{n}", 0x1F3A5F)
    for c in range(8):
        grid[(3, c)] = logged(f"button-{c + 1}", f"B{c + 1}", RAINBOW[c], 0x000000)
    return "TEST", grid


def media_page():
    grid = {
        (0, 0): button(labelled("VOL"), down=streamd("/volume/mute"),
                       left=streamd("/volume/down"), right=streamd("/volume/up")),
        (1, 0): button(labelled("TRACK"), up=[mediakey("play")], held=ymusic("/like"),
                       left=[mediakey("prev"), *streamd("/nowplaying/prev")],
                       right=[mediakey("next"), *streamd("/nowplaying/next")]),
        (2, 0): button(labelled("SEEK"), left=[post(SEEK + "/seek/back")], right=[post(SEEK + "/seek/forward")]),
        (0, 7): button(labelled("SUN"), down=brightness("/brightness/hold"), up=brightness("/brightness/release"),
                       left=brightness("/brightness/down"), right=brightness("/brightness/up")),
        **{(r, 1): volume_cell(r) for r in range(3)},
        (1, 2): play_pause(),
        (2, 2): call_tile("call_mic", "mic"),
        (2, 3): call_tile("call_cam", "cam"),
        (2, 4): day_tile(),
        (2, 5): clock(),
        **{(r, 6): ai_strip(r) for r in range(3)},
        (1, 7): ai_switch(),
        (2, 7): button(labelled("SCROLL"), down=[post(SCROLL + "/scroll/down")], up=[post(SCROLL + "/scroll/up")],
                       left=[post(SCROLL + "/scroll/back")], right=[post(SCROLL + "/scroll/forward")]),
        (1, 5): agents_tile(),
        (0, 2): input_tile(),
        (1, 3): emoji_tile(),
        (0, 3): wifi_tile(),
        (0, 4): headphones_tile(),
        (0, 5): awake_tile(),
        (3, 5): agents_led(),
        (3, 6): dictation_led(),
        (3, 1): call_led("call_mic"),
        (3, 2): call_led("call_cam"),
        (3, 3): hangup_led(),
        (3, 0): kvm_led(),
    }
    if WINDOWS:
        for cell in ((3, 0), (1, 3), (0, 2), (0, 3), (0, 4), (0, 5), (2, 3), (2, 4), (3, 2), (3, 3), (3, 6)):
            del grid[cell]
    else:
        for c in range(8):
            grid[(3, c)] = choice_gated(c + 1, grid.get((3, c)) or blank())
    return "MEDIA", grid


def blank():
    return button([canvas(), box("box0", 0x000000)])


def export(name, grid):
    controls = {}
    for r in range(4):
        for c in range(8):
            grid.setdefault((r, c), blank())
    for (r, c), control in sorted(grid.items()):
        controls.setdefault(str(r), {})[str(c)] = control
    return {
        "version": 12,
        "type": "page",
        "page": {
            "id": nid(),
            "name": name,
            "controls": controls,
            "gridSize": {"minColumn": 0, "maxColumn": 7, "minRow": 0, "maxRow": 3},
            "pageVariables": [],
        },
        "instances": {},
        "connectionCollections": [],
        "oldPageNumber": 1,
        "imageLibrary": [],
        "imageLibraryCollections": [],
    }


def control_id(r, c):
    if r == 3:
        return f"button-{c + 1}"
    return f"encoder-{r}-{c}" if c in (0, 7) else f"button-{r}-{c}"


def hold_controls(grid):
    return sorted(control_id(r, c) for (r, c), b in grid.items()
                  if str(HOLD_MS) in b["steps"]["0"]["action_sets"])


def write(path, data):
    with open(path, "w") as f:
        json.dump(data, f, indent=1)


out = sys.argv[1]
for build in (test_page, media_page):
    rng.seed(build.__name__)
    name, grid = build()
    write(os.path.join(out, f"{name.lower()}-page.companionconfig"), export(name, grid))
    if build is media_page:
        with open(os.path.join(out, "hold-controls.txt"), "w") as f:
            f.write(",".join(hold_controls(grid)) + "\n")
write(os.path.join(out, "custom-variables.json"), CUSTOM_VARIABLES)
