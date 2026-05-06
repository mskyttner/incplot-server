IMAGE   ?= incplot-server
TAG     ?= local
PORT    ?= 8080
SERVER  ?= http://localhost:$(PORT)

.PHONY: build run demo smoketest gallery

# Kill any container already using the port, then start a fresh named one.
_stop_port:
	@-podman ps --format "{{.ID}} {{.Ports}}" | awk -v p=":$(PORT)->" '$$2 ~ p {print $$1}' | xargs -r podman rm -f 2>/dev/null || true

build:
	podman build -f Containerfile -t $(IMAGE):$(TAG) .

run: _stop_port
	podman run --rm --name $(IMAGE) -p $(PORT):8080 $(IMAGE):$(TAG)

demo: _stop_port
	@echo "Starting container..."
	@podman run --rm -d -p $(PORT):8080 --name incplot-demo $(IMAGE):$(TAG) > /dev/null
	@echo "Waiting for server..."
	@until curl -sf http://localhost:$(PORT)/incplot/sources > /dev/null; do sleep 0.5; done
	@echo "Running all chart types:"
	@bash smoketest.sh $(SERVER) || true
	@echo ""
	@echo "UI: http://localhost:$(PORT)/incplot/ui"
	@echo "Press Ctrl-C to stop."
	@podman wait incplot-demo > /dev/null 2>&1 || podman stop incplot-demo > /dev/null 2>&1

gallery: _stop_port
	@podman run --rm -d -p $(PORT):8080 --name incplot-gallery $(IMAGE):$(TAG) > /dev/null
	@until curl -sf http://localhost:$(PORT)/incplot/sources > /dev/null; do sleep 0.5; done
	quarto render gallery.qmd -P server:$(SERVER)
	@podman stop incplot-gallery > /dev/null 2>&1 || true

smoketest:
	@bash smoketest.sh $(SERVER)
