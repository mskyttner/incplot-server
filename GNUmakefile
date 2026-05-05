IMAGE   ?= incplot-server
TAG     ?= local
PORT    ?= 8080
SERVER  ?= http://localhost:$(PORT)

.PHONY: build run demo smoketest

build:
	podman build -f Containerfile -t $(IMAGE):$(TAG) .

run:
	podman run --rm -p $(PORT):8080 $(IMAGE):$(TAG)

demo:
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

smoketest:
	@bash smoketest.sh $(SERVER)
