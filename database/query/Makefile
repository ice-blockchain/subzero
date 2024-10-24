export TESTDB := .testdata/testdb_3M.sqlite3

.PHONY: test gendb benchdb_select benchdb_insert all

all: test

gendb:
	GENDB=yes go test -v -timeout=10h -run 'TestGenerateDataForFile3M'

benchdb_select: $(TESTDB)
	BENCHDB=yes go test -v -timeout=10h -run=^$$ -bench=^BenchmarkSelectBy -benchtime=20x -count 20 -benchmem . | tee $@_current.txt

BENCH_INSERT_P_LIST   := 1 10 100
BENCH_INSERT_DB_LIST  := testdb_3M

benchdb_insert:
	set -e; \
	for db in $(BENCH_INSERT_DB_LIST); do \
		for p in $(BENCH_INSERT_P_LIST); do \
			echo "===> Parallelism: $${p}"; \
			BENCHDB=yes \
			BENCHDB_PARALLELISM=$${p} \
			TESTDB=".testdata/$${db}.sqlite3" \
				go test -v -timeout=10h -run=^$$ -bench=^BenchmarkEventInsert -benchtime=30s -count 1 -benchmem .  | tee $@_current.txt; \
		done | tee $${db}_results.txt; \
	done;

test:
	go test -v -timeout=10h
