BIN ?= $(HOME)/.local/bin
LIB ?= $(HOME)/.local/lib
SHARE ?= $(HOME)/.local/share/zored-stream
AGENT = com.zored.stream.streamd
AGENT_PLIST = $(HOME)/Library/LaunchAgents/$(AGENT).plist
DOMAIN = gui/$(shell id -u)

.PHONY: mediakey streamd streamd-uninstall pages deploy

mediakey:
	mkdir -p $(BIN)
	swiftc -O tools/mediakey/main.swift -o $(BIN)/mediakey

streamd:
	mkdir -p $(LIB) $(SHARE) $(HOME)/Library/Logs/zored-stream
	clang -dynamiclib -arch arm64 -arch arm64e -fobjc-arc -framework Foundation -framework CoreAudio \
		-framework AudioToolbox -o $(LIB)/libstreamd.dylib tools/streamd/streamd.m
	cp tools/streamd/streamd.pl $(SHARE)/streamd.pl
	sed 's|@HOME@|$(HOME)|g' tools/streamd/$(AGENT).plist > $(AGENT_PLIST)
	if launchctl print $(DOMAIN)/$(AGENT) >/dev/null 2>&1; then launchctl bootout $(DOMAIN)/$(AGENT); fi
	launchctl bootstrap $(DOMAIN) $(AGENT_PLIST)

streamd-uninstall:
	if launchctl print $(DOMAIN)/$(AGENT) >/dev/null 2>&1; then launchctl bootout $(DOMAIN)/$(AGENT); fi
	rm -f $(AGENT_PLIST)

pages:
	python3 -I tools/gen_pages.py companion

deploy: pages
	node tools/deploy.mjs --vars companion/custom-variables.json companion/media-page.companionconfig:1 companion/test-page.companionconfig:2
