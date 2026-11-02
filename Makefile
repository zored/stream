BIN ?= $(HOME)/.local/bin
LIB ?= $(HOME)/.local/lib
SHARE ?= $(HOME)/.local/share/zored-stream
AGENTS = $(HOME)/Library/LaunchAgents
DOMAIN = gui/$(shell id -u)
MODULE ?= $(abspath ../companion-surface-loupedeck)
MODULES = $(SHARE)/companion-modules
HAPTIC ?= SHORT_LOWER
HAPTIC_HOLD_MS ?= 400
HAPTIC_SLIDE ?= SHORT_LOWER
HAPTIC_STEP ?= 10
SIGN ?= zored-stream

define install-agent
	mkdir -p $(HOME)/Library/Logs/zored-stream
	sed 's|@HOME@|$(HOME)|g' tools/$(1)/com.zored.stream.$(1).plist > $(AGENTS)/com.zored.stream.$(1).plist
	if launchctl print $(DOMAIN)/com.zored.stream.$(1) >/dev/null 2>&1; then launchctl kickstart -k $(DOMAIN)/com.zored.stream.$(1); \
	else launchctl bootstrap $(DOMAIN) $(AGENTS)/com.zored.stream.$(1).plist; fi
endef

define uninstall-agent
	if launchctl print $(DOMAIN)/com.zored.stream.$(1) >/dev/null 2>&1; then launchctl bootout $(DOMAIN)/com.zored.stream.$(1); fi
	rm -f $(AGENTS)/com.zored.stream.$(1).plist
endef

.PHONY: build mediakey streamd-bin streamd streamd-uninstall zored_stream-cert zored_stream-bin zored_stream zored_stream-uninstall ymusic-bin ymusic ymusic-uninstall module companion companion-restart pages deploy

build:
	$(MAKE) BIN=$(CURDIR)/build LIB=$(CURDIR)/build SIGN=- mediakey streamd-bin ymusic-bin zored_stream-bin

mediakey:
	mkdir -p $(BIN)
	swiftc -O tools/mediakey/main.swift -o $(BIN)/mediakey

streamd-bin:
	mkdir -p $(LIB)
	clang -dynamiclib -arch arm64 -arch arm64e -fobjc-arc -framework Foundation -framework CoreAudio \
		-framework AudioToolbox -o $(LIB)/libstreamd.dylib tools/streamd/streamd.m

streamd: streamd-bin
	mkdir -p $(SHARE)
	cp tools/streamd/streamd.pl $(SHARE)/streamd.pl
	$(call install-agent,streamd)

streamd-uninstall:
	$(call uninstall-agent,streamd)

zored_stream-cert:
	tools/zored_stream/cert.sh

zored_stream-bin:
	mkdir -p $(BIN)
	cd tools/zored_stream && GOWORK=off go build -o $(BIN)/zored_stream .
	codesign -f -s $(SIGN) -i com.zored.stream.zored_stream $(BIN)/zored_stream

zored_stream: zored_stream-bin
	$(call install-agent,zored_stream)

zored_stream-uninstall:
	$(call uninstall-agent,zored_stream)

ymusic-bin:
	mkdir -p $(BIN)
	swiftc -O tools/ymusic/main.swift -o $(BIN)/ymusic

ymusic: ymusic-bin
	$(call install-agent,ymusic)

ymusic-uninstall:
	$(call uninstall-agent,ymusic)

module:
	test -d $(MODULE) || git clone -b zored_stream git@github.com:zored/companion-surface-loupedeck.git $(MODULE)
	cd $(MODULE) && corepack yarn install && corepack yarn build
	mkdir -p $(MODULES)
	ln -sfn $(MODULE) $(MODULES)/loupedeck
	python3 -I tools/companion_config.py enable_developer=true dev_modules_path=$(MODULES)
	$(MAKE) companion-restart
	node tools/surface.mjs dev hapticPressPattern=$(HAPTIC) hapticHoldMs=$(HAPTIC_HOLD_MS) \
		hapticSlidePattern=$(HAPTIC_SLIDE) hapticSlideStep=$(HAPTIC_STEP)

companion:
	python3 -I tools/companion_config.py start_minimised=true
	osascript -e 'tell application "System Events" to if not (exists login item "Companion") then make login item at end with properties {path:"/Applications/Companion.app", hidden:true}'

companion-restart:
	osascript -e 'quit app "Companion"'
	while pgrep -xq Companion; do sleep 0.5; done
	open -g -a Companion
	until curl -sfo /dev/null http://127.0.0.1:8000; do sleep 1; done

pages:
	python3 -I tools/gen_pages.py companion

deploy: pages
	node tools/deploy.mjs --vars companion/custom-variables.json companion/media-page.companionconfig:1 companion/test-page.companionconfig:2
