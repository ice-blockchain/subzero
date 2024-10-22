export TESTDB := .testdata/testdb_3M.sqlite3

.PHONY: test gendb benchdb_select benchdb_insert all

all: test

gendb:
	GENDB=yes go test -v -timeout=10h -run 'TestGenerateDataForFile3M'

benchdb_select: $(TESTDB)
	BENCHDB=yes go test -v -timeout=10h -run=^$$ -bench=^BenchmarkSelectBy -benchtime=20x -count 20 -benchmem . | tee $@_current.txt

benchdb_insert:
	BENCHDB=yes go test -v -timeout=10h -run=^$$ -bench=^BenchmarkEventInsert -benchtime=30s -benchmem . | tee $@_current.txt

test:
	go test -v -timeout=10h
