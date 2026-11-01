import json
import secrets
import string
import sys

ALPHABET = string.ascii_letters + string.digits + "_-"
RAINBOW = [0xFF0000, 0xFF8000, 0xFFFF00, 0x00FF00, 0x00FFFF, 0x0000FF, 0x8000FF, 0xFF00FF]


def nid():
    return "".join(secrets.choice(ALPHABET) for _ in range(21))


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


def log(message):
    return {"type": "action", "id": nid(), "connectionId": "internal", "definitionId": "custom_log",
            "options": {"message": v(message)}, "upgradeIndex": None}


def button(name, text, bg, fg=0xFFFFFF, rotary=False):
    sets = {"down": [log(f"TEST {name} down")], "up": [log(f"TEST {name} up")]}
    if rotary:
        sets["rotate_left"] = [log(f"TEST {name} left")]
        sets["rotate_right"] = [log(f"TEST {name} right")]
    return {
        "type": "button-layered",
        "options": {"stepProgression": "auto", "stepExpression": "", "rotaryActions": rotary,
                    "canModifyStyleInApis": True, "notes": ""},
        "style": {"layers": layers(text, bg, fg)},
        "feedbacks": [],
        "steps": {"0": {"action_sets": sets, "options": {"runWhileHeld": []}}},
        "localVariables": [],
    }


def controls():
    grid = {str(r): {} for r in range(4)}
    for r in range(3):
        grid[str(r)]["0"] = button(f"knob-L{r + 1}", f"KL{r + 1}", 0, rotary=True)
        grid[str(r)]["7"] = button(f"knob-R{r + 1}", f"KR{r + 1}", 0, rotary=True)
        grid[str(r)]["1"] = button(f"strip-L{r + 1}", f"SL{r + 1}", 0x303030)
        grid[str(r)]["6"] = button(f"strip-R{r + 1}", f"SR{r + 1}", 0x303030)
        for c in range(2, 6):
            n = r * 4 + c - 1
            grid[str(r)][str(c)] = button(f"tile-{n}", f"T{n}", 0x1F3A5F)
    for c in range(8):
        grid["3"][str(c)] = button(f"button-{c + 1}", f"B{c + 1}", RAINBOW[c], 0x000000)
    return grid


page = {
    "version": 12,
    "type": "page",
    "page": {
        "id": nid(),
        "name": "TEST",
        "controls": controls(),
        "gridSize": {"minColumn": 0, "maxColumn": 7, "minRow": 0, "maxRow": 3},
        "pageVariables": [],
    },
    "instances": {},
    "connectionCollections": [],
    "oldPageNumber": 1,
    "imageLibrary": [],
    "imageLibraryCollections": [],
}

json.dump(page, open(sys.argv[1], "w"), indent=1)
