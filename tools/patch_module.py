import glob
import os
import re
import sys

HAPTIC = int(os.environ.get("HAPTIC", "0x33"), 0)
HOLD_MS = 400
MARKER = "/*zored-stream*/"
HOLD_MARKER = "/*zored-hold*/"
STREAMD = "http://127.0.0.1:47113"
MODULES = os.path.expanduser("~/Library/Application Support/companion/surfaces/loupedeck-*/main.js")
VIBRATE = f'{MARKER}this.#e.vibrate({HAPTIC}).catch(V=>this.#t.warn("vibrate failed: "+V))'
HOLD = f'{HOLD_MARKER}(clearTimeout(this.__zh),this.__zh=setTimeout(()=>{VIBRATE},{HOLD_MS}))'
DRAG = (
    'let o=a.changedTouches.find(l=>l.target.screen==y.Left);'
    'if(o){let p=this.__zy??o.y;this.__zy=o.y;'
    f'if(p!==o.y)fetch("{STREAMD}/volume/drag?dy="+(p-o.y),{{method:"POST"}})'
    '.catch(V=>this.#t.warn("drag failed: "+V))}'
)

PATCHES = [
    ('this.#e.on("down",a=>{s.keyDownById(a.id)})',
     f'this.#e.on("down",a=>{{{VIBRATE},s.keyDownById(a.id)}})'),
    ('if(o.target.control!==void 0&&o.target.screen===y.Center)s.keyDownById(o.target.control.id);else if(o.target.screen==y.Wheel)',
     f'if(o.target.control!==void 0&&o.target.screen===y.Center)({VIBRATE},s.keyDownById(o.target.control.id));else if(o.target.screen==y.Wheel)'),
    ('this.#n&&this.#e.on("touchmove",a=>{if(this.#r!=="slider")return;',
     f'this.#n&&this.#e.on("touchmove",a=>{{if(this.#r!=="slider"){{{DRAG}return}}'),
    ('this.#e.on("touchend",a=>{',
     'this.#e.on("touchend",a=>{this.__zy=void 0;'),
]

HOLD_PATCHES = [
    ('s.keyDownById(a.id)}),this.#e.on("up",a=>{s.keyUpById(a.id)})',
     f'{HOLD},s.keyDownById(a.id)}}),this.#e.on("up",a=>{{clearTimeout(this.__zh),s.keyUpById(a.id)}})'),
    ('s.keyDownById(o.target.control.id));else if(o.target.screen==y.Wheel)',
     f'{HOLD},s.keyDownById(o.target.control.id));else if(o.target.screen==y.Wheel)'),
    ('this.#e.on("touchend",a=>{this.__zy=void 0;',
     'this.#e.on("touchend",a=>{this.__zy=void 0;clearTimeout(this.__zh);'),
]


def apply(path, source, patches):
    for old, new in patches:
        if source.count(old) != 1:
            sys.exit(f"{path}: expected exactly one match for {old[:60]}…, module changed?")
        source = source.replace(old, new)
    return source


paths = sys.argv[1:] or glob.glob(MODULES)
if not paths:
    sys.exit(f"no Loupedeck surface module at {MODULES}")
for path in paths:
    original = open(path, encoding="utf-8").read()
    source = original if MARKER in original else apply(path, original, PATCHES)
    source = source if HOLD_MARKER in source else apply(path, source, HOLD_PATCHES)
    source = re.sub(r"(/\*zored-stream\*/this\.#e\.vibrate\()(\d+)", rf"\g<1>{HAPTIC}", source)
    if source != original:
        open(path, "w", encoding="utf-8").write(source)
    print(f"{'patched' if source != original else 'up to date'}, haptic {HAPTIC:#04x}, hold {HOLD_MS} ms: {path}")
