// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/alitto/pond/v2"
	"github.com/cockroachdb/errors"
	combinations "github.com/mxschmitt/golang-combinations"
	"github.com/nbd-wtf/go-nostr"
	"github.com/schollz/progressbar/v3"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type structElement struct {
	// Name of the field.
	Name []string
	// Full index of the field.
	Addr []int
	// Number of elements to generate for this slice, if applicable.
	NumElem int
}

func (f *structElement) Clone() *structElement {
	if f == nil {
		return &structElement{}
	}

	return &structElement{
		Name:    append([]string{}, f.Name...),
		Addr:    append([]int{}, f.Addr...),
		NumElem: f.NumElem,
	}
}

func (f *structElement) GetName() string {
	return strings.Join(f.Name, ".")
}

func (f *structElement) GetAddress() []int {
	return f.Addr
}

func helperParseFilterStruct(t *testing.T, typ reflect.Type, parent *structElement) (fields []*structElement) {
	t.Helper()

	for i := range typ.NumField() {
		field := typ.Field(i)
		switch field.Type.Kind() {
		case reflect.Struct:
			el := parent.Clone()
			el.Name = append(el.Name, field.Name)
			el.Addr = append(el.Addr, field.Index...)
			s := helperParseFilterStruct(t, field.Type, el)
			fields = append(fields, s...)

		case reflect.Slice, reflect.Ptr, reflect.Int, reflect.Map:
			el := parent.Clone()
			el.Name = append(el.Name, field.Name)
			el.Addr = append(el.Addr, field.Index...)
			el.NumElem = 1
			fields = append(fields, el)
			if field.Type.Kind() == reflect.Slice {
				next := el.Clone()
				next.NumElem = int(rand.Int32N(6)) + 1
				fields = append(fields, next)
			}

		case reflect.String:
			for _, v := range []string{"Images", "Quotes", "References", "Videos", "Expiration", "Search"} {
				el := parent.Clone()
				el.Name = append(el.Name, v)
				el.Addr = append(el.Addr, field.Index...)
				fields = append(fields, el)
			}
		}
	}

	return fields
}

func helperRandomBool(t *testing.T) string {
	t.Helper()

	if rand.Int64N(100)%2 == 0 {
		return "true"
	}

	return "false"
}

func helperNewFilterFromElements(t *testing.T, fields []*structElement) model.Filter {
	t.Helper()

	var f model.Filter
	for _, field := range fields {
		value := reflect.ValueOf(&f).Elem().FieldByIndex(field.GetAddress())
		switch field.GetName() {
		case "Authors", "IDs":
			vals := make([]string, field.NumElem)
			for i := range field.NumElem {
				vals[i] = generateHexString()
			}
			value.Set(reflect.ValueOf(vals))

		case "Kinds":
			vals := make([]int, field.NumElem)
			for i := range field.NumElem {
				vals[i] = generateKind()
			}
			value.Set(reflect.ValueOf(vals))

		case "Tags":
			vals := make([]string, field.NumElem)
			for i := range field.NumElem {
				vals[i] = generateHexString()
			}
			m := model.TagMap{}.SetLiterals("e", vals...)

			value.Set(reflect.ValueOf(m))

		case "Limit":
			l := int(rand.Int64N(100))
			value.Set(reflect.ValueOf(l))

		case "Until", "Since":
			ts := model.Timestamp(generateCreatedAt())
			value.Set(reflect.ValueOf(&ts))

		case "Search":
			val := value.String()
			val += ` "` + generateRandomString(rand.IntN(20)) + `"`
			value.Set(reflect.ValueOf(val))

		case "Expiration", "Videos", "Images", "Quotes", "References":
			val := value.String()
			if val != "" {
				val += " "
			}
			val += field.GetName() + ":" + helperRandomBool(t)
			value.Set(reflect.ValueOf(val))

		case "Addresses":
			// Skip this field.

		default:
			t.Fatalf("unknown field: %s", field.GetName())
		}
	}

	helperBenchEnsureValidRange(t, &f)

	return f
}

func helperGenFilterCombinations(t *testing.T) [][]*structElement {
	t.Helper()

	var filter model.Filter

	fields := helperParseFilterStruct(t, reflect.TypeOf(filter), nil)
	sets := combinations.All(fields)
	t.Logf("found %d total combination(s)", len(sets))

	slices.SortStableFunc(sets, func(i, j []*structElement) int {
		if len(i) < len(j) {
			return -1
		}
		if len(i) > len(j) {
			return 1
		}
		return 0
	})

	return sets
}

func TestQueryFuzzWhereGenerator(t *testing.T) {
	t.Parallel()

	db, _ := helperEnsureDatabaseWithData(t, 100)
	defer db.Close()

	sets := helperGenFilterCombinations(t)
	bar := progressbar.Default(int64(len(sets)), "testing where sets")
	w := min(runtime.NumCPU()*3, 100)
	t.Run("Fuzz", func(t *testing.T) {
		t.Logf("testing %d sets with %d workers", len(sets), w)
		pool := pond.NewPool(w)
		errCh := make(chan error, len(sets))
		for i, set := range sets {
			i, set := i, set
			pool.Submit(func() {
				filter := helperNewFilterFromElements(t, set)
				_, err := db.CountEvents(t.Context(), filter)
				if err != nil {
					errCh <- errors.Errorf("failed to count events for set #%d (%#v): %w", i+1, filter, err)
				}
				bar.Add(1)
			})
		}
		pool.StopAndWait()
		close(errCh)

		for err := range errCh {
			require.NoError(t, err)
		}
	})
}

type Plan struct {
	Plans     []Plan   `json:"Plans"`
	NodeType  string   `json:"Node Type"`
	IndexName string   `json:"Index Name"`
	IndexCond string   `json:"Index Cond"`
	SortKey   []string `json:"Sort Key"`
}
type Query struct {
	Plan     Plan    `json:"Plan"`
	ExecTime float64 `json:"Execution Time"`
}

func helperPlanConsume(t *testing.T, plan *Plan, ops map[string]int) {
	t.Helper()

	if plan.NodeType != "" {
		ops[plan.NodeType]++
	}
	for _, p := range plan.Plans {
		helperPlanConsume(t, &p, ops)
	}
}

func helperPlanHas(t *testing.T, plan *Plan, op string) bool {
	t.Helper()

	if plan.NodeType == op {
		return true
	}
	for _, p := range plan.Plans {
		if helperPlanHas(t, &p, op) {
			return true
		}
	}
	return false
}

func helperQueryHas(t *testing.T, q []Query, op string) bool {
	t.Helper()

	for _, p := range q {
		if helperPlanHas(t, &p.Plan, op) {
			return true
		}
	}
	return false
}

func TestQueryFuzzIndexes(t *testing.T) {
	t.Parallel()

	if os.Getenv("CI") != "" {
		t.Skip("skipping test on CI")
	}

	db := helperNewDatabase(t)
	defer db.Close()
	helperFillDatabase(t, db, 11000)

	op := make(map[string]int)
	sets := helperGenFilterCombinations(t)
	results := make([]Query, 0, len(sets))
	w := min(runtime.NumCPU()*3, 100)

	t.Run("Fuzz", func(t *testing.T) {
		t.Logf("testing %d sets with %d workers", len(sets), w)
		bar := progressbar.Default(int64(len(sets)), "testing sets")
		pool := pond.NewPool(w)
		resultsCh := make(chan []Query, len(sets))
		errCh := make(chan error, len(sets))

		for i, set := range sets {
			i, set := i, set
			pool.Submit(func() {
				defer bar.Add(1)
				filter := helperNewFilterFromElements(t, set)
				sql, params, err := db.generateSelectEventsSQL(t.Context(), filter)
				if err != nil {
					errCh <- errors.Errorf("failed to generate select events sql for set #%d (%#v): %w", i+1, set, err)
					return
				}

				sql = "EXPLAIN (FORMAT JSON, ANALYZE) " + sql
				result, err := connector.GetNamed[string](t.Context(), db.db, sql, params)
				if err != nil {
					errCh <- errors.Errorf("failed to execute query for set #%d: %w", i+1, err)
					return
				}
				if result == nil {
					errCh <- errors.Errorf("nil result for set #%d", i+1)
					return
				}

				var q []Query
				err = json.Unmarshal([]byte(*result), &q)
				if err != nil {
					errCh <- errors.Errorf("failed to unmarshal query result for set #%d: %w", i+1, err)
					return
				}

				resultsCh <- q

				if helperQueryHas(t, q, "Seq Scan") {
					var emptyFilter model.Filter
					if !nostr.FilterEqual(filter, emptyFilter) {
						errCh <- errors.Errorf("set #%d: found SCAN without INDEX; sql: %s; params: %#v", i+1, sql, params)
					}
				}
			})
		}

		pool.StopAndWait()
		close(resultsCh)
		close(errCh)

		for q := range resultsCh {
			results = append(results, q...)
		}

		for err := range errCh {
			t.Errorf("error: %v", err)
		}
	})

	for _, q := range results {
		helperPlanConsume(t, &q.Plan, op)
	}

	t.Run("OpSummary", func(t *testing.T) {
		keys := make([]string, 0, len(op))
		for k := range op {
			keys = append(keys, k)
		}
		slices.SortStableFunc(keys, func(i, j string) int {
			if op[i] > op[j] {
				return -1
			}
			if op[i] < op[j] {
				return 1
			}
			return 0
		})
		t.Log("Operations Summary:")
		for _, k := range keys {
			t.Logf("%s: %d", k, op[k])
		}
	})
}
