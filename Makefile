GO ?= go
INSTALL ?= install
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
INSTALL_NVIM ?= 1
NVIM_CONFIG_DIR ?= $(if $(XDG_CONFIG_HOME),$(XDG_CONFIG_HOME),$(HOME)/.config)/nvim
NVIM_PLUGIN_DIR ?= $(PREFIX)/share/lazymusic/nvim

.PHONY: build test test-nvim vet install uninstall help

build:
	CGO_ENABLED=1 $(GO) build -buildvcs=false -trimpath -o bin/lazymusic ./cmd/lazymusic

test:
	CGO_ENABLED=1 $(GO) test -count=1 -timeout=120s ./...

test-nvim:
	nvim --headless -u NONE -i NONE -l tests/nvim_spec.lua

vet:
	CGO_ENABLED=1 $(GO) vet ./...

install: build
	$(INSTALL) -d "$(DESTDIR)$(BINDIR)"
	$(INSTALL) -m 0755 bin/lazymusic "$(DESTDIR)$(BINDIR)/lazymusic"
ifeq ($(INSTALL_NVIM),1)
	$(INSTALL) -d "$(DESTDIR)$(NVIM_PLUGIN_DIR)/lua/lazymusic" "$(DESTDIR)$(NVIM_PLUGIN_DIR)/doc"
	$(INSTALL) -m 0644 lua/lazymusic/init.lua "$(DESTDIR)$(NVIM_PLUGIN_DIR)/lua/lazymusic/init.lua"
	$(INSTALL) -m 0644 docs/lazymusic.txt "$(DESTDIR)$(NVIM_PLUGIN_DIR)/doc/lazymusic.txt"
	bin/lazymusic setup-nvim --config-dir "$(DESTDIR)$(NVIM_CONFIG_DIR)" --plugin-dir "$(NVIM_PLUGIN_DIR)" --binary "$(BINDIR)/lazymusic" --help-dir "$(DESTDIR)$(NVIM_PLUGIN_DIR)/doc"
endif

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/lazymusic"

help:
	@printf '%s\n' \
	  'make build      Build bin/lazymusic (requires CGO)' \
	  'make install    Install binary, bundled Neovim plugin and lazy.nvim config' \
	  'make uninstall  Remove the installed binary; preserve config and sessions' \
	  'make test       Run unit tests' \
	  'make test-nvim  Test the Neovim plugin (requires nvim 0.10+)' \
	  'make vet        Run Go static checks' \
	  '' \
	  'Overrides: GO, INSTALL, PREFIX, BINDIR, DESTDIR' \
	  'Neovim: NVIM_CONFIG_DIR, NVIM_PLUGIN_DIR, INSTALL_NVIM=0 to skip' \
	  'Example: make install PREFIX=/path/to/prefix'
