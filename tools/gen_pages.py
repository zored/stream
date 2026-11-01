import json
import os
import random
import string
import sys

ALPHABET = string.ascii_letters + string.digits + "_-"
RAINBOW = [0xFF0000, 0xFF8000, 0xFFFF00, 0x00FF00, 0x00FFFF, 0x0000FF, 0x8000FF, 0xFF00FF]
MEDIAKEY = "$HOME/.local/bin/mediakey"
CUSTOM_VARIABLES = {"volume": "0", "volume_until": "0"}
VOLUME_VISIBLE = "$(internal:time_unix) <= $(custom:volume_until)"
rng = random.Random()


def nid():
    return "".join(rng.choice(ALPHABET) for _ in range(21))


def v(value):
    return {"value": value, "isExpression": False}


def expr(value):
    return {"value": value, "isExpression": True}


def geometry(enabled=None, y=None, height=None):
    return {"enabled": enabled or v(True), "opacity": v(100), "x": v(0), "y": y or v(0), "width": v(100),
            "height": height or v(100), "rotation": v(0)}


def canvas(decoration="default"):
    return {"id": "canvas", "name": "Canvas", "usage": "auto", "type": "canvas",
            "decoration": v(decoration), "showStatusIcons": v("default")}


def box(layer_id, color, **geo):
    return {"id": layer_id, "name": layer_id, "usage": "auto", "type": "box", "pinnedProperties": ["color"],
            **geometry(**geo), "color": v(color), "cornerRadius": v(0), "borderWidth": v(0), "borderColor": v(0),
            "borderPosition": v("inside")}


def text(layer_id, value, color=0xFFFFFF, **geo):
    return {"id": layer_id, "name": layer_id, "usage": "auto", "type": "text",
            "pinnedProperties": ["text", "fontsize", "fontsizeAllowShrink", "color", "halign", "valign"],
            **geometry(**geo), "text": v(value), "color": v(color), "halign": v("center"), "valign": v("center"),
            "fontsize": v(100), "fontsizeAllowShrink": v(True), "font": v("companion-sans"), "weight": v("normal"),
            "styles": v([]), "outlineColor": v(4278190080)}


def labelled(label, bg, fg=0xFFFFFF, decoration="default"):
    return [canvas(decoration), box("box0", bg), text("text0", label, fg)]


def action(definition, options):
    return {"type": "action", "id": nid(), "connectionId": "internal", "definitionId": definition,
            "options": {k: x if isinstance(x, dict) else v(x) for k, x in options.items()}, "upgradeIndex": None}


def log(message):
    return action("custom_log", {"message": message})


def sh(command, target=None):
    return action("exec", {"path": command, "cwd": "", "timeout": 2000, "targetVariable": target})


def mediakey(key):
    return sh(f"{MEDIAKEY} {key}")


def volume(key):
    return [
        sh(f"{MEDIAKEY} {key}", target="volume"),
        action("custom_variable_set_value",
               {"name": "volume_until", "create": True, "value": expr("$(internal:time_unix) + 1")}),
    ]


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
    fill = f"max(0, min(100, ($(custom:volume) - {base:.2f}) * 3))"
    visible = expr(VOLUME_VISIBLE)
    style = [
        canvas("none"),
        box("track", 0x202020, enabled=visible),
        box("fill", 0x3FA9F5, enabled=visible, y=expr(f"100 - {fill}"), height=expr(fill)),
    ]
    if row == 1:
        style.append(text("level", "$(custom:volume)", enabled=visible))
    return button(style)


def clock():
    return button([canvas("none"), box("box0", 0x101010),
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
    blue, green = 0x1F3A5F, 0x1E6B32
    return "MEDIA", {
        (0, 0): button(labelled("VOL", 0), down=volume("mute"), left=volume("voldown"), right=volume("volup")),
        (0, 7): button(labelled("TRACK", 0), down=[mediakey("play")], left=[mediakey("prev")], right=[mediakey("next")]),
        **{(r, 1): volume_cell(r) for r in range(3)},
        (0, 2): button(labelled("PREV", blue), down=[mediakey("prev")]),
        (0, 3): button(labelled("PLAY\nPAUSE", green), down=[mediakey("play")]),
        (0, 4): button(labelled("NEXT", blue), down=[mediakey("next")]),
        (0, 5): clock(),
        (3, 0): button(labelled("PREV", blue), down=[mediakey("prev")]),
        (3, 1): button(labelled("PLAY", green), down=[mediakey("play")]),
        (3, 2): button(labelled("NEXT", blue), down=[mediakey("next")]),
    }


def export(name, grid):
    controls = {}
    for (r, c), control in grid.items():
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
