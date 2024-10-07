// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	combinations "github.com/mxschmitt/golang-combinations"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

type filterElement struct {
	// Name of the field.
	Name []string
	// Full index of the field.
	Addr []int
}

func (f *filterElement) Clone() *filterElement {
	if f == nil {
		return &filterElement{}
	}

	return &filterElement{
		Name: append([]string{}, f.Name...),
		Addr: append([]int{}, f.Addr...),
	}
}

func (f *filterElement) GetName() string {
	return strings.Join(f.Name, ".")
}

func (f *filterElement) GetAddress() []int {
	return f.Addr
}

func helperParseFilterStruct(t *testing.T, typ reflect.Type, parent *filterElement) (fields []*filterElement) {
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
			fields = append(fields, el)

		case reflect.String:
			for _, v := range []string{"Images", "Quotes", "References", "Videos", "Expiration"} {
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

	if rand.Int63n(100)%2 == 0 {
		return "true"
	}

	return "false"
}

func helperNewFilterFromElements(t *testing.T, fields []*filterElement) model.Filter {
	t.Helper()

	var f model.Filter
	for _, field := range fields {
		value := reflect.ValueOf(&f).Elem().FieldByIndex(field.GetAddress())
		switch field.GetName() {
		case "Authors", "IDs":
			n := rand.Int31n(4)
			vals := make([]string, n)
			for i := range n {
				vals[i] = generateHexString()
			}
			value.Set(reflect.ValueOf(vals))

		case "Kinds":
			k := []int{generateKind()}
			value.Set(reflect.ValueOf(k))

		case "Tags":
			values := []string{}
			for range rand.Intn(3) {
				values = append(values, generateHexString())
			}
			m := model.TagMap{
				"e": values,
			}
			value.Set(reflect.ValueOf(m))

		case "Limit":
			l := int(rand.Int63n(100))
			value.Set(reflect.ValueOf(l))

		case "Until", "Since":
			ts := model.Timestamp(generateCreatedAt())
			value.Set(reflect.ValueOf(&ts))

		case "Expiration", "Videos", "Images", "Quotes", "References":
			val := value.String()
			if val != "" {
				val += " "
			}
			val += field.GetName() + ":" + helperRandomBool(t)
			value.Set(reflect.ValueOf(val))

		default:
			t.Fatalf("unknown field: %s", field.GetName())
		}
	}

	helperBenchEnsureValidRange(t, &f)

	return f
}

func TestQueryFuzzWhereGenerator(t *testing.T) {
	t.Parallel()

	var sets [][]*filterElement
	t.Run("PrepareSets", func(t *testing.T) {
		var filter model.Filter

		fields := helperParseFilterStruct(t, reflect.TypeOf(filter), nil)
		sets = combinations.All(fields)
		t.Logf("found %d total combination(s)", len(sets))
	})

	db := helperNewDatabase(t)
	defer db.Close()
	helperFillDatabase(t, db, 100)

	t.Run("Fuzz", func(t *testing.T) {
		for i, set := range sets {
			filter := helperNewFilterFromElements(t, set)
			_, err := db.CountEvents(context.TODO(), &model.Subscription{Filters: model.Filters{filter}})
			require.NoErrorf(t, err, "failed to count events for set #%d (%#v)", i+1, filter)
		}
	})
}

func TestQueryFuzzNoUseTempBTREE(t *testing.T) {
	t.Parallel()

	var sets [][]*filterElement
	t.Run("PrepareSets", func(t *testing.T) {
		var filter model.Filter

		fields := helperParseFilterStruct(t, reflect.TypeOf(filter), nil)
		sets = combinations.All(fields)
		t.Logf("found %d total combination(s)", len(sets))
		slices.SortStableFunc(sets, func(i, j []*filterElement) int {
			if len(i) < len(j) {
				return -1
			}
			if len(i) > len(j) {
				return 1
			}
			return 0
		})
	})

	db := helperNewDatabase(t)
	defer db.Close()
	helperFillDatabase(t, db, 100)

	op := make(map[string]int)

	t.Run("Fuzz", func(t *testing.T) {
		for i, set := range sets {
			sql, params, err := generateSelectEventsSQL(&model.Subscription{Filters: model.Filters{helperNewFilterFromElements(t, set)}}, 0, 100)
			require.NoErrorf(t, err, "failed to generate select events sql for set #%d (%#v)", i+1, set)

			sql = "EXPLAIN QUERY PLAN " + sql
			stmt, err := db.prepare(context.Background(), sql, hashSQL(sql))
			require.NoError(t, err)

			rows, err := stmt.QueryContext(context.Background(), params)
			require.NoError(t, err)
			for rows.Next() {
				var s1, s2, s3, s4 string
				err := rows.Scan(&s1, &s2, &s3, &s4)
				require.NoError(t, err)
				op[s4]++
				if s4 == "USE TEMP B-TREE FOR ORDER BY" {
					t.Logf("set #%d: %s (%+v)", i+1, sql, params)
					t.Log(s1, s2, s3, s4)
					t.FailNow()
				}
			}
			rows.Close()
		}
	})

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
