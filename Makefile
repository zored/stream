BIN ?= $(HOME)/.local/bin

.PHONY: mediakey pages deploy

mediakey:
	mkdir -p $(BIN)
	swiftc -O tools/mediakey/main.swift -o $(BIN)/mediakey

pages:
	python3 -I tools/gen_pages.py companion

deploy: pages
	node tools/deploy.mjs companion/media-page.companionconfig:1 companion/test-page.companionconfig:2
