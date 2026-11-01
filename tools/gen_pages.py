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
ICONS = os.path.join(os.path.dirname(os.path.abspath(__file__)), "icons")
CUSTOM_VARIABLES = {name: "" for name in (
    "volume", "volume_anim", "volume_alpha", "np_playing", "np_title", "np_artist", "np_track", "call_mic", "call_cam")}
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


def image(layer_id, data, **geo):
    return {"id": layer_id, "name": layer_id, "usage": "auto", "type": "image",
            "pinnedProperties": ["base64Image", "halign", "valign"], **geometry(**geo), "base64Image": v(data),
            "halign": v("center"), "valign": v("center"), "fillMode": v("fit")}


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


def button(style, down=(), up=(), left=None, right=None):
    sets = {"down": list(down), "up": list(up)}
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
        "steps": {"0": {"action_sets": sets, "options": {"runWhileHeld": []}}},
        "localVariables": [],
    }


def logged(name, label, bg, fg=0xFFFFFF, rotary=False):
    events = {"down": [log(f"TEST {name} down")], "up": [log(f"TEST {name} up")]}
    if rotary:
        events["left"] = [log(f"TEST {name} left")]
        events["right"] = [log(f"TEST {name} right")]
    return button(labelled(label, bg, fg), **events)


def volume_cell(row):
    base = (2 - row) * 100 / 3
    fill = f"max(0, min(100, ($(custom:volume_anim) - {base:.2f}) * 3))"
    visible = expr(VOLUME_ALPHA)
    cell_w, cell_h, strip_h = 60, 90, 270
    center_y = strip_h / 2 - cell_h * row
    style = [
        canvas(),
        text("np", "$(custom:np_track)", fontsize=28, opacity=expr(f"100 - {VOLUME_ALPHA}"), rotation=v(90),
             x=v(round((cell_w / 2 - strip_h / 2) / cell_w * 100, 2)), width=v(round(strip_h / cell_w * 100, 2)),
             y=v(round((center_y - cell_w / 2) / cell_h * 100, 2)), height=v(round(cell_w / cell_h * 100, 2))),
        box("track", 0x202020, opacity=visible),
        box("fill", 0x3FA9F5, opacity=visible, y=expr(f"100 - {fill}"), height=expr(fill)),
    ]
    if row == 1:
        style.append(text("level", "$(custom:volume)", fontsize=14, opacity=visible))
    return button(style)


def play_pause():
    return button([
        canvas(),
        box("box0", 0x000000),
        text("icon", expr(f'{PLAYING} ? "⏸" : "▶"'), fontsize=110),
    ], down=[mediakey("play")])


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
        "unknown": f"$(internal:time_unix) % 2 == 0 ? {0xFFB000} : 0",
    }))], **call_actions(var))


def call_tile(var, name):
    state = f"$(custom:{var})"
    return button([
        canvas(),
        box("box0", by_state(var, {"on": 0x1B5E20, "off": 0xB3261E, "unknown": 0x8A6100})),
        image("on", icon(name), opacity=expr(f'{state} == "on" ? 100 : 0'), x=v(20), y=v(20), width=v(60), height=v(60)),
        image("off", icon(f"{name}-off"), opacity=expr(f'{state} == "off" ? 100 : 0'), x=v(20), y=v(20), width=v(60), height=v(60)),
        text("unknown", "?", fontsize=60, opacity=expr(f'{state} == "unknown" ? 100 : 0')),
    ], **call_actions(var))


def clock():
    return button([canvas(), box("box0", 0x000000),
                   text("text0", "$(internal:time_hm)\n$(internal:date_d).$(internal:date_m)")])


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
        (1, 0): button(labelled("TRACK"), down=[mediakey("play")], left=[mediakey("prev")], right=[mediakey("next")]),
        **{(r, 1): volume_cell(r) for r in range(3)},
        (0, 2): play_pause(),
        (2, 2): call_tile("call_mic", "mic"),
        (2, 3): call_tile("call_cam", "cam"),
        (2, 5): clock(),
        (3, 1): call_led("call_mic"),
        (3, 2): call_led("call_cam"),
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
