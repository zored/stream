#!/bin/sh
set -eu

name=$1 plist=$2 domain=$3
svc=$domain/com.zored.stream.$name
dst=$HOME/Library/LaunchAgents/com.zored.stream.$name.plist

loaded() { launchctl print "$svc" >/dev/null 2>&1; }

running() { launchctl print "$svc" 2>/dev/null | grep -m1 -E '^[[:space:]]*state = ' | grep -q 'state = running'; }

unload() {
	loaded || return 0
	launchctl bootout "$svc" || true
	i=0
	while loaded; do
		i=$((i + 1))
		[ "$i" -le 50 ] || { echo "launch_agent: $svc still loaded after bootout" >&2; exit 1; }
		sleep 0.2
	done
}

load() { launchctl bootstrap "$domain" "$dst"; }

wait_running() {
	i=0
	while [ "$i" -lt 30 ]; do
		running && return 0
		i=$((i + 1))
		sleep 0.2
	done
	return 1
}

if cmp -s "$plist" "$dst"; then
	if loaded; then launchctl kickstart -k "$svc"; else load; fi
else
	unload
	cp "$plist" "$dst"
	load
fi

wait_running && exit 0
echo "launch_agent: $svc not running, reloading" >&2
unload
load
wait_running && exit 0
launchctl print "$svc" | grep -E '^[[:space:]]*(state|last exit code|job state) = ' >&2 || true
log show --last 1m --style compact --predicate "process == \"launchd\" AND eventMessage CONTAINS \"com.zored.stream.$name\"" 2>/dev/null | tail -5 >&2 || true
echo "launch_agent: $svc failed to start" >&2
exit 1
