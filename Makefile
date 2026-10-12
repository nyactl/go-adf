.PHONY: test lint fuzz

FUZZTIME ?= 1m

test:
	go test -race ./...

lint:
	golangci-lint run ./...

fuzz:
	for f in FuzzFromMarkdown FuzzToMarkdown FuzzRoundTrip FuzzADFRoundTrip; do \
		go test -run '^$$' -fuzz "^$$f$$" -fuzztime $(FUZZTIME) . || exit 1; \
	done
