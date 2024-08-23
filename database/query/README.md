## TODO
- Optimize time range queries, avg time now is 500ms (to fetch 2500 events)
  - BenchmarkSelectByCreatedAtRange
  - BenchmarkSelectByKindAndCreatedAtRange

## Comparing benchmark results
- Install [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
- Run
```shell
$ benchstat old_results.txt new_results.txt
```
