GO ?= go
INSTALL ?= install
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=

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

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/lazymusic"

help:
	@printf '%s\n' \
	  'make build      Build bin/lazymusic (requires CGO)' \
	  'make install    Build and install to BINDIR (default: ~/.local/bin)' \
	  'make uninstall  Remove the installed binary; preserve config and sessions' \
	  'make test       Run unit tests' \
	  'make test-nvim  Test the Neovim plugin (requires nvim 0.10+)' \
	  'make vet        Run Go static checks' \
	  '' \
	  'Overrides: GO, INSTALL, PREFIX, BINDIR, DESTDIR' \
	  'Example: make install PREFIX=/path/to/prefix'
