import glob
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DAEMON = os.path.join(ROOT, "tools", "zored_stream")
MAC_ONLY_PORTS = {"47114": "/mic", "47115": None, "47122": None, "47123": None, "47124": None}
WINDOWS_PLANNED = ("/key/",)
COMPANION_SET = {"ctl_brightness"}
errors = []


def fail(message):
    errors.append(message)


def actions(control):
    for step in control.get("steps", {}).values():
        for key, items in step["action_sets"].items():
            stack = list(items)
            while stack:
                a = stack.pop()
                stack.extend(a.get("children", {}).get("default", []))
                yield key, a


def routes():
    go = "".join(open(p).read() for p in sorted(glob.glob(os.path.join(DAEMON, "*.go"))) if not p.endswith("_test.go"))
    main = open(os.path.join(DAEMON, "main.go")).read()
    ports = {flag: port for flag, port in re.findall(r'(\w+)Addr := flag\.String\("\w+", "127\.0\.0\.1:(\d+)"', main)}
    out = {}
    for flag, body in re.findall(r"serve\(\*(\w+)Addr, func\(mux \*http\.ServeMux\) \{(.*?)\n\t\}\)", main, re.S):
        paths = set(re.findall(r'handle(?:Value)?\(mux, "([^"]+)"', body))
        paths |= set(re.findall(r'HandleFunc\("(?:POST|GET) ([^"{]+)', body))
        out[ports[flag]] = paths
    if set(out) != set(ports.values()):
        fail(f"main.go: ports without a serve block: {set(ports.values()) - set(out)}")
    if len(set(ports.values())) != len(ports):
        fail(f"main.go: duplicate ports {ports}")
    return go, out


def check(directory, windows, go, served):
    variables = json.load(open(os.path.join(directory, "custom-variables.json")))
    for page in sorted(glob.glob(os.path.join(directory, "*.companionconfig"))):
        name = os.path.relpath(page, ROOT)
        data = json.load(open(page))
        for used in sorted(set(re.findall(r"\$\(custom:(\w+)\)", json.dumps(data)))):
            if used not in variables:
                fail(f"{name}: $(custom:{used}) is not in custom-variables.json")
        media = data["page"]["name"] == "MEDIA"
        for r, row in data["page"]["controls"].items():
            for c, control in row.items():
                for key, a in actions(control):
                    if a["definitionId"] != "exec":
                        continue
                    command = a["options"]["path"]["value"]
                    where = f"{name} {r}/{c} {key}"
                    if "curl" in command and " -m 1 " not in command:
                        fail(f"{where}: curl without -m 1: {command}")
                    for port, path in re.findall(r"http://127\.0\.0\.1:(\d+)(/[\w/]*)", command):
                        if port not in served:
                            fail(f"{where}: no daemon module on :{port}")
                        elif windows and path.startswith(WINDOWS_PLANNED):
                            pass
                        elif path not in served[port] and path.rsplit("/", 1)[0] + "/" not in served[port]:
                            fail(f"{where}: :{port} has no route {path}")
                        if windows and port in MAC_ONLY_PORTS and not (MAC_ONLY_PORTS[port] and path.startswith(MAC_ONLY_PORTS[port])):
                            fail(f"{where}: mac-only :{port}{path} on the windows profile")
                    if media and r == "3" and not windows and "/choice/" not in command:
                        fail(f"{where}: button not gated by choice mode")
                    if media and r == "3" and not windows and not re.search(r"\|\| \{ .+ ; \}$", command):
                        fail(f"{where}: choice gate without a fallback")
    if not windows:
        published = set(re.findall(r'\.set\("(\w+)"', go)) - {"choice_"}
        published |= set(re.findall(r'"(call_\w+)": ', go)) | COMPANION_SET
        published |= {f"choice_{b}" for b in range(1, 9)}
        for name in sorted(published - set(variables)):
            fail(f"daemon sets {name}, missing from custom-variables.json")
        for name in sorted(set(variables) - published):
            fail(f"custom-variables.json has {name}, the daemon never sets it")


def check_hooks():
    source = open(os.path.join(ROOT, "tools", "agent_hooks.py")).read()
    for needle in ("curl -s -m 1 ", '--noproxy "*"', "|| true", '"async": True'):
        if needle not in source:
            fail(f"tools/agent_hooks.py: hooks must stay fail-safe, missing {needle}")


check_hooks()
go, served = routes()
check(os.path.join(ROOT, "companion"), False, go, served)
check(os.path.join(ROOT, "companion", "windows"), True, go, served)
for e in errors:
    print(e)
sys.exit(1 if errors else 0)
