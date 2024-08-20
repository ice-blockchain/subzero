export TESTDB := .testdata/testdb_3M.sqlite3

.PHONY: test gendb benchdb benchcompare

gendb:
	GENDB=yes go test -v -timeout=10h -run 'TestGenerateDataForFile3M'

benchdb: $(TESTDB)
	BENCHDB=yes go test -v -timeout=10h -run=^$$ -bench=^BenchmarkSelectBy -benchtime=20x -count 20 -benchmem . | tee benchdb_current.txt

test:
	go test -v -timeout=10h

.PHONY: benchdb_current.txt
benchdb_current.txt:
	@if [ ! -e $@ ]; then \
		echo "No benchmark results found"; \
		echo "Please run 'make benchdb' to generate test database and run benchmarks"; \
		false; \
	fi

# https://pkg.go.dev/golang.org/x/perf/cmd/benchstat
benchcompare: .testdata/bench_results.txt benchdb_current.txt
	@echo "Comparing current benchmark results with previous results ($<)"
	benchstat $^
