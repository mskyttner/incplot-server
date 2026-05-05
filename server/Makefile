BINARY     := incplot-server
INSTALL    := $(HOME)/.local/bin/$(BINARY)
INCPLOT    := $(HOME)/.local/bin/incplot
INCPLOT_SRC:= $(abspath ../stage/bin/incplot)

.PHONY: all build install run dev smoketest clean

all: install

build: $(BINARY)

$(BINARY): $(wildcard *.go) $(wildcard data/*)
	go build -o $@ .

install: $(BINARY)
	cp $(BINARY) $(INSTALL)
	@echo "installed $(INSTALL)"

# Copy the incplot plotting binary from the CMake stage directory.
install-incplot:
	@test -f $(INCPLOT_SRC) || (echo "error: $(INCPLOT_SRC) not found — run cmake build first" && exit 1)
	cp $(INCPLOT_SRC) $(INCPLOT)
	@echo "installed $(INCPLOT)"

run: install
	INCPLOT_BIN=$(INCPLOT) $(INSTALL)

# Build and run the local binary without installing — the running binary is
# always the freshly-built artifact in this directory.
dev: $(BINARY)
	INCPLOT_BIN=$(INCPLOT) ./$(BINARY)

# Run the dynamic-source example script against a live server.
# Override the server URL with: make smoketest SERVER=http://host:port
SERVER ?= http://localhost:8080
smoketest:
	@bash $(abspath ../example-dynamic-source.sh) $(SERVER)

clean:
	rm -f $(BINARY)

ASCIINEMA_VERSION := 3.15.1
ASCIINEMA_BASE    := https://github.com/asciinema/asciinema-player/releases/download/v$(ASCIINEMA_VERSION)

assets: assets/asciinema-player.min.js assets/asciinema-player.css

assets/asciinema-player.min.js:
	mkdir -p assets
	curl -fsSLo $@ $(ASCIINEMA_BASE)/asciinema-player.min.js

assets/asciinema-player.css:
	mkdir -p assets
	curl -fsSLo $@ $(ASCIINEMA_BASE)/asciinema-player.css
