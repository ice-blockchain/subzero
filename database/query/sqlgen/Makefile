.PHONY: all
all: sqlgen

.PHONY: sqlgen
sqlgen: $(wildcard *.go)
	CGO_ENABLED=0 go build -tags test -v -o $@

.PHONY: clean
clean:
	rm -f sqlgen
