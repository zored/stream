import json
import os
import random
import string
import sys

ALPHABET = string.ascii_letters + string.digits + "_-"
RAINBOW = [0xFF0000, 0xFF8000, 0xFFFF00, 0x00FF00, 0x00FFFF, 0x0000FF, 0x8000FF, 0xFF00FF]
MEDIAKEY = "$HOME/.local/bin/mediakey"
rng = random.Random()


def nid():
    return "".join(rng.choice(ALPHABET) for _ in range(21))


def v(value):
    return {"value": value, "isExpression": False}


def geometry():
    return {"enabled": v(True), "opacity": v(100), "x": v(0), "y": v(0), "width": v(100), "height": v(100), "rotation": v(0)}


def layers(text, bg, fg):
    return [
        {"id": "canvas", "name": "Canvas", "usage": "auto", "type": "canvas",
         "decoration": v("default"), "showStatusIcons": v("default")},
        {"id": "box0", "name": "Background", "usage": "auto", "type": "box", "pinnedProperties": ["color"],
         **geometry(), "color": v(bg), "cornerRadius": v(0), "borderWidth": v(0), "borderColor": v(0),
         "borderPosition": v("inside")},
        {"id": "text0", "name": "Text", "usage": "auto", "type": "text",
         "pinnedProperties": ["text", "fontsize", "fontsizeAllowShrink", "color", "halign", "valign"],
         **geometry(), "text": v(text), "color": v(fg), "halign": v("center"), "valign": v("center"),
         "fontsize": v(100), "fontsizeAllowShrink": v(True), "font": v("companion-sans"), "weight": v("normal"),
         "styles": v([]), "outlineColor": v(4278190080)},
    ]


def action(definition, options):
    return {"type": "action", "id": nid(), "connectionId": "internal", "definitionId": definition,
            "options": {k: v(x) for k, x in options.items()}, "upgradeIndex": None}


def log(message):
    return action("custom_log", {"message": message})


def sh(command):
    return action("exec", {"path": command, "cwd": "", "timeout": 2000, "targetVariable": None})


def mediakey(key):
    return sh(f"{MEDIAKEY} {key}")


def button(text, bg, fg=0xFFFFFF, down=(), up=(), left=None, right=None):
    sets = {"down": list(down), "up": list(up)}
    rotary = left is not None or right is not None
    if rotary:
        sets["rotate_left"] = list(left or [])
        sets["rotate_right"] = list(right or [])
    return {
        "type": "button-layered",
        "options": {"stepProgression": "auto", "stepExpression": "", "rotaryActions": rotary,
                    "canModifyStyleInApis": True, "notes": ""},
        "style": {"layers": layers(text, bg, fg)},
        "feedbacks": [],
        "steps": {"0": {"action_sets": sets, "options": {"runWhileHeld": []}}},
        "localVariables": [],
    }


def logged(name, text, bg, fg=0xFFFFFF, rotary=False):
    events = {"down": [log(f"TEST {name} down")], "up": [log(f"TEST {name} up")]}
    if rotary:
        events["left"] = [log(f"TEST {name} left")]
        events["right"] = [log(f"TEST {name} right")]
    return button(text, bg, fg, **events)


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
    blue, green, red, grey = 0x1F3A5F, 0x1E6B32, 0x8B1E1E, 0x303030
    return "MEDIA", {
        (0, 0): button("VOL", 0, down=[mediakey("mute")], left=[mediakey("voldown")], right=[mediakey("volup")]),
        (0, 7): button("TRACK", 0, down=[mediakey("play")], left=[mediakey("prev")], right=[mediakey("next")]),
        (0, 2): button("PREV", blue, down=[mediakey("prev")]),
        (0, 3): button("PLAY\nPAUSE", green, down=[mediakey("play")]),
        (0, 4): button("NEXT", blue, down=[mediakey("next")]),
        (0, 5): button("MUTE", red, down=[mediakey("mute")]),
        (1, 2): button("VOL -", grey, down=[mediakey("voldown")]),
        (1, 3): button("VOL +", grey, down=[mediakey("volup")]),
        (3, 0): button("PREV", blue, down=[mediakey("prev")]),
        (3, 1): button("PLAY", green, down=[mediakey("play")]),
        (3, 2): button("NEXT", blue, down=[mediakey("next")]),
        (3, 7): button("MUTE", red, down=[mediakey("mute")]),
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


out = sys.argv[1]
for build in (test_page, media_page):
    rng.seed(build.__name__)
    name, grid = build()
    with open(os.path.join(out, f"{name.lower()}-page.companionconfig"), "w") as f:
        json.dump(export(name, grid), f, indent=1)
