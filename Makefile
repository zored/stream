BIN ?= $(HOME)/.local/bin
LIB ?= $(HOME)/.local/lib
SHARE ?= $(HOME)/.local/share/zored-stream
AGENTS = $(HOME)/Library/LaunchAgents
DOMAIN = gui/$(shell id -u)
HAPTIC ?= 0x33

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

.PHONY: mediakey streamd streamd-uninstall vkcall vkcall-uninstall patch-module pages deploy

mediakey:
	mkdir -p $(BIN)
	swiftc -O tools/mediakey/main.swift -o $(BIN)/mediakey

streamd:
	mkdir -p $(LIB) $(SHARE)
	clang -dynamiclib -arch arm64 -arch arm64e -fobjc-arc -framework Foundation -framework CoreAudio \
		-framework AudioToolbox -o $(LIB)/libstreamd.dylib tools/streamd/streamd.m
	cp tools/streamd/streamd.pl $(SHARE)/streamd.pl
	$(call install-agent,streamd)

streamd-uninstall:
	$(call uninstall-agent,streamd)

vkcall:
	mkdir -p $(BIN)
	cd tools/vkcall && GOWORK=off go build -o $(BIN)/vkcall .
	$(call install-agent,vkcall)

vkcall-uninstall:
	$(call uninstall-agent,vkcall)

patch-module:
	HAPTIC=$(HAPTIC) python3 -I tools/patch_module.py

pages:
	python3 -I tools/gen_pages.py companion

deploy: pages
	node tools/deploy.mjs --vars companion/custom-variables.json companion/media-page.companionconfig:1 companion/test-page.companionconfig:2
