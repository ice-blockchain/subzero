export TESTDB := .testdata/testdb_3M.sqlite3

.PHONY: test gendb benchdb benchcompare all

all: test

gendb:
	GENDB=yes go test -v -timeout=10h -run 'TestGenerateDataForFile3M'

benchdb: $(TESTDB)
	BENCHDB=yes go test -v -timeout=10h -run=^$$ -bench=^BenchmarkSelectBy -benchtime=20x -count 20 -benchmem . | tee benchdb_current.txt

test:
	go test -v -timeout=10h
