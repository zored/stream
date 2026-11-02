import json
import os
import sys

PATH = os.path.expanduser("~/Library/Application Support/companion/config.json")

config = json.load(open(PATH, encoding="utf-8"))
for pair in sys.argv[1:]:
    key, sep, value = pair.partition("=")
    if not sep:
        sys.exit(f"expected key=value, got {pair!r}")
    if key not in config:
        sys.exit(f"{PATH}: unknown key {key!r}")
    config[key] = json.loads(value) if value in ("true", "false") else value
json.dump(config, open(PATH, "w", encoding="utf-8"), indent="\t")
print(f"{PATH}: {', '.join(sys.argv[1:])}")
