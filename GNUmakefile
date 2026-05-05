IMAGE   ?= incplot-server
TAG     ?= local
PORT    ?= 8080

.PHONY: build run demo

build:
	podman build -f Containerfile -t $(IMAGE):$(TAG) .

run:
	podman run --rm -p $(PORT):8080 $(IMAGE):$(TAG)

demo:
	@echo "Open http://localhost:$(PORT)/incplot/ui"
	podman run --rm -p $(PORT):8080 $(IMAGE):$(TAG)
