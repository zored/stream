import base64
import json
import os
import random
import string
import sys

ALPHABET = string.ascii_letters + string.digits + "_-"
RAINBOW = [0xFF0000, 0xFF8000, 0xFFFF00, 0x00FF00, 0x00FFFF, 0x0000FF, 0x8000FF, 0xFF00FF]
MEDIAKEY = "$HOME/.local/bin/mediakey"
STREAMD = "http://127.0.0.1:47113"
VKCALL = "http://127.0.0.1:47114"
YMUSIC = "http://127.0.0.1:47115"
AI = "http://127.0.0.1:47117"
HOLD_MS = 400
ICONS = os.path.join(os.path.dirname(os.path.abspath(__file__)), "icons")
CUSTOM_VARIABLES = {name: "" for name in (
    "volume", "volume_anim", "volume_alpha", "np_playing", "np_title", "np_artist", "np_track", "np_svg", "np_app", "ym_like", "call_mic", "call_cam",
    "ai_strip_svg", "ai_session", "ai_session_reset", "ai_day", "ai_week", "ai_week_reset", "ai_level",
    "ai_model", "ai_effort", "ai_state")}
VOLUME_ALPHA = "$(custom:volume_alpha)"
PLAYING = "$(custom:np_playing) == 1"
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
    return sh(f"{MEDIAKEY} {key}")


def post(url):
    return sh(f"curl -fsS -m 1 -X POST {url}")


def streamd(path):
    return [post(STREAMD + path)]


def vkcall(path):
    return [post(VKCALL + path)]


def ymusic(path):
    return [post(YMUSIC + path)]


def ai(path):
    return [post(f"'{AI}{path}'")]


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


def volume_cell(row):
    base = (2 - row) * 100 / 3
    fill = f"max(0, min(100, ($(custom:volume_anim) - {base:.2f}) * 3))"
    visible = expr(VOLUME_ALPHA)
    return button([
        canvas(),
        strip_image("np", "np_svg", row, 90, expr(f"100 - {VOLUME_ALPHA}")),
        box("track", 0x202020, opacity=visible),
        box("fill", 0xFFFFFF, opacity=visible, y=expr(f"100 - {fill}"), height=expr(fill)),
    ])


def play_pause():
    return button([
        canvas(),
        box("box0", 0x000000),
        text("icon", expr(f'{PLAYING} ? "⏸" : "▶"'), fontsize=110),
    ], up=[mediakey("play")], held=ymusic("/like"))


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


def hangup_tile():
    return button([
        canvas(),
        box("box0", 0x000000),
        image("icon", icon("hangup"), opacity=expr(f"{IN_CALL} ? 100 : 0"), x=v(15), y=v(15), width=v(70), height=v(70)),
    ], down=vkcall("/call/end"))


TIME_SVG = ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 -74 256 77"><text font-family="Helvetica" '
            'font-size="100" fill="#fff"', "/text></svg>")


def b64(value):
    return base64.b64encode(value.encode()).decode()


def lookup(table, var):
    return f'substr("{"".join(map(b64, table))}", {var} * 4, {var} * 4 + 4)'


def time_image():
    head, tail = TIME_SVG
    head += " " * (-(len(head) + 1) % 3) + ">"
    hours = lookup([f"{h:02d}:" for h in range(24)], "$(internal:time_h)")
    minutes = lookup([f"{m:02d}<" for m in range(60)], "$(internal:time_m)")
    return expr(f'concat("data:image/svg+xml;base64,{b64(head)}", {hours}, {minutes}, "{b64(tail)}")')


def clock():
    return button([canvas(), box("box0", 0x000000),
                   image("time", time_image(), "fill", x=v(6), y=v(6), width=v(88), height=v(58)),
                   text("date", "$(internal:date_d).$(internal:date_m)", 0x808080, fontsize=80, y=v(66), height=v(32))])


def ai_switch():
    return button(labelled("AI"), down=ai("/ai/down"), up=ai("/ai/up"), left=ai("/ai/turn?d=-1"), right=ai("/ai/turn?d=1"))


def ai_strip(row):
    return button([canvas(), box("box0", 0x000000),
                   image("strip", expr("$(custom:ai_strip_svg)"), "fill", y=v(-100 * row), height=v(300))])


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
    return "MEDIA", {
        (0, 0): button(labelled("VOL"), down=streamd("/volume/mute"),
                       left=streamd("/volume/down"), right=streamd("/volume/up")),
        (1, 0): button(labelled("TRACK"), up=[mediakey("play")], held=ymusic("/like"),
                       left=[mediakey("prev")], right=[mediakey("next")]),
        **{(r, 1): volume_cell(r) for r in range(3)},
        (1, 2): play_pause(),
        (2, 2): call_tile("call_mic", "mic"),
        (2, 3): call_tile("call_cam", "cam"),
        (2, 4): hangup_tile(),
        (2, 5): clock(),
        **{(r, 6): ai_strip(r) for r in range(3)},
        (1, 7): ai_switch(),
        (3, 1): call_led("call_mic"),
        (3, 2): call_led("call_cam"),
        (3, 3): hangup_led(),
    }


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


def write(path, data):
    with open(path, "w") as f:
        json.dump(data, f, indent=1)


out = sys.argv[1]
for build in (test_page, media_page):
    rng.seed(build.__name__)
    name, grid = build()
    write(os.path.join(out, f"{name.lower()}-page.companionconfig"), export(name, grid))
write(os.path.join(out, "custom-variables.json"), CUSTOM_VARIABLES)
