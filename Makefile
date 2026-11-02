BIN ?= $(HOME)/.local/bin
LIB ?= $(HOME)/.local/lib
SHARE ?= $(HOME)/.local/share/zored-stream
AGENTS = $(HOME)/Library/LaunchAgents
DOMAIN = gui/$(shell id -u)
MODULE ?= $(abspath ../companion-surface-loupedeck)
MODULES = $(SHARE)/companion-modules
HAPTIC ?= SHORT_LOWER
HAPTIC_HOLD_MS ?= 400
HAPTIC_HOLD_CONTROLS ?= encoder-1-0
HAPTIC_SLIDE ?= SHORT_LOWER
HAPTIC_STEP ?= 10
SIGN ?= zored-stream
USAGE_PROXY ?= http://squid.ai-proxy.vk.team:8083

define install-agent
	mkdir -p $(HOME)/Library/Logs/zored-stream
	sed 's|@HOME@|$(HOME)|g; s|@USAGE_PROXY@|$(USAGE_PROXY)|g' tools/$(1)/com.zored.stream.$(1).plist > $(AGENTS)/com.zored.stream.$(1).plist
	if launchctl print $(DOMAIN)/com.zored.stream.$(1) >/dev/null 2>&1; then launchctl kickstart -k $(DOMAIN)/com.zored.stream.$(1); \
	else launchctl bootstrap $(DOMAIN) $(AGENTS)/com.zored.stream.$(1).plist; fi
endef

define uninstall-agent
	if launchctl print $(DOMAIN)/com.zored.stream.$(1) >/dev/null 2>&1; then launchctl bootout $(DOMAIN)/com.zored.stream.$(1); fi
	rm -f $(AGENTS)/com.zored.stream.$(1).plist
endef

.PHONY: build mediakey zored_stream-cert zored_stream-bin zored_stream zored_stream-uninstall module companion companion-restart ymusic-launcher pages deploy agent-hooks

build:
	$(MAKE) BIN=$(CURDIR)/build LIB=$(CURDIR)/build SHARE=$(CURDIR)/build SIGN=- mediakey zored_stream-bin

mediakey:
	mkdir -p $(BIN)
	swiftc -O tools/mediakey/main.swift -o $(BIN)/mediakey

zored_stream-cert:
	tools/zored_stream/cert.sh

zored_stream-bin:
	mkdir -p $(BIN) $(LIB) $(SHARE)
	clang -dynamiclib -arch arm64 -arch arm64e -fobjc-arc -framework Foundation -framework ImageIO -framework CoreGraphics \
		-o $(LIB)/libnowplaying.dylib tools/zored_stream/nowplaying/nowplaying.m
	cp tools/zored_stream/nowplaying/nowplaying.pl $(SHARE)/nowplaying.pl
	cd tools/zored_stream && GOWORK=off go build -o $(BIN)/zored_stream .
	codesign -f -s $(SIGN) -i com.zored.stream.zored_stream $(BIN)/zored_stream

zored_stream: zored_stream-bin
	$(call install-agent,zored_stream)

zored_stream-uninstall:
	$(call uninstall-agent,zored_stream)

module:
	test -d $(MODULE) || git clone -b zored_stream git@github.com:zored/companion-surface-loupedeck.git $(MODULE)
	cd $(MODULE) && corepack yarn install && corepack yarn build
	mkdir -p $(MODULES)
	ln -sfn $(MODULE) $(MODULES)/loupedeck
	python3 -I tools/companion_config.py enable_developer=true dev_modules_path=$(MODULES)
	$(MAKE) companion-restart
	node tools/surface.mjs dev hapticPressPattern=$(HAPTIC) hapticHoldMs=$(HAPTIC_HOLD_MS) hapticHoldControls=$(HAPTIC_HOLD_CONTROLS) \
		hapticSlidePattern=$(HAPTIC_SLIDE) hapticSlideStep=$(HAPTIC_STEP)

YMUSIC_APP = /Applications/Яндекс Музыка.app
YMUSIC_FLAGS = --disable-backgrounding-occluded-windows --disable-renderer-backgrounding --disable-background-timer-throttling

ymusic-launcher:
	mkdir -p $(HOME)/Applications
	osacompile -o "$(HOME)/Applications/Яндекс Музыка (stream).app" \
		-e 'do shell script "open -a \"$(YMUSIC_APP)\" --args $(YMUSIC_FLAGS)"'

companion:
	python3 -I tools/companion_config.py start_minimised=true
	osascript -e 'tell application "System Events" to if not (exists login item "Companion") then make login item at end with properties {path:"/Applications/Companion.app", hidden:true}'

companion-restart:
	osascript -e 'quit app "Companion"'
	while pgrep -xq Companion; do sleep 0.5; done
	open -g -a Companion
	until curl -sfo /dev/null http://127.0.0.1:8000; do sleep 1; done

agent-hooks:
	python3 -I tools/agent_hooks.py

pages:
	python3 -I tools/gen_pages.py companion
	python3 -I tools/gen_pages.py companion/windows windows

deploy: pages
	node tools/deploy.mjs --vars companion/custom-variables.json companion/media-page.companionconfig:1 companion/test-page.companionconfig:2
