import json
import os
import sys

PATH = os.path.expanduser(sys.argv[1] if len(sys.argv) > 1 else "~/.claude/settings.json")
URL = "http://127.0.0.1:47120/agent/event"
COMMAND = ('curl -s -m 1 --noproxy "*" -o /dev/null -H "Agent-Pid: $PPID" -H "Agent-Host: ${CLAUDE_CODE_HOST_SESSION_ID-}" '
           '-H "Agent-Iterm: ${ITERM_SESSION_ID-}" -H "Agent-App: ${__CFBundleIdentifier-}" '
           f'--data-binary @- {URL} || true')
EVENTS = {"SessionStart": None, "SessionEnd": None, "UserPromptSubmit": None, "Stop": None, "StopFailure": None,
          "Notification": None, "PermissionRequest": None, "PreToolUse": "AskUserQuestion", "PostToolUse": None}

raw = open(PATH, encoding="utf-8").read() if os.path.exists(PATH) else "{}\n"
settings = json.loads(raw)
hooks = settings.setdefault("hooks", {})
for event in list(hooks):
    hooks[event] = [m for m in hooks[event] if not any(URL in h.get("command", "") for h in m.get("hooks", []))]
for event, matcher in EVENTS.items():
    entry = {"matcher": matcher} if matcher else {}
    entry["hooks"] = [{"type": "command", "command": COMMAND, "async": True}]
    hooks.setdefault(event, []).append(entry)
with open(PATH, "w", encoding="utf-8") as f:
    f.write(json.dumps(settings, indent=2, ensure_ascii=False) + "\n")
print(f"{PATH}: agent hooks on {', '.join(EVENTS)}")
