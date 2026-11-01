import glob
import os
import sys

HAPTIC = 0x01
MARKER = "/*zored-stream*/"
STREAMD = "http://127.0.0.1:47113"
MODULES = os.path.expanduser("~/Library/Application Support/companion/surfaces/loupedeck-*/main.js")
VIBRATE = f'{MARKER}this.#e.vibrate({HAPTIC}).catch(V=>this.#t.warn("vibrate failed: "+V))'
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

paths = sys.argv[1:] or glob.glob(MODULES)
if not paths:
    sys.exit(f"no Loupedeck surface module at {MODULES}")
for path in paths:
    source = open(path, encoding="utf-8").read()
    if MARKER in source:
        print(f"already patched: {path}")
        continue
    for old, new in PATCHES:
        if source.count(old) != 1:
            sys.exit(f"{path}: expected exactly one match for {old[:60]}…, module changed?")
        source = source.replace(old, new)
    open(path, "w", encoding="utf-8").write(source)
    print(f"patched: {path}")
