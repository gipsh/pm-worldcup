BINARY := pm-worldcup
SLUG ?= fifwc-usa-par-2026-06-12

.PHONY: build test run clean

build:
	go build -o $(BINARY) .

test:
	go test ./...

run: build
	./$(BINARY) -slug $(SLUG)

clean:
	rm -f $(BINARY)
