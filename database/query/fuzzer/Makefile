.PHONY: all
all: fuzzer

.PHONY: fuzzer
fuzzer: $(wildcard *.go)
	CGO_ENABLED=0 go build -tags test -v -o $@

.PHONY: clean
clean:
	rm -f fuzzer
