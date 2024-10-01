// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"reflect"
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
		}
	}

	return fields
}

func helperRandomBool(t *testing.T) *bool {
	t.Helper()

	val := rand.Int63n(100)%2 == 0

	return &val
}

func helperNewFilterFromElements(t *testing.T, fields []*filterElement) model.Filter {
	t.Helper()

	var f model.Filter
	for _, field := range fields {
		value := reflect.ValueOf(&f).Elem().FieldByIndex(field.GetAddress())
		switch field.GetName() {
		case "Filter.Authors", "Filter.IDs":
			n := rand.Int31n(4)
			vals := make([]string, n)
			for i := range n {
				vals[i] = generateHexString()
			}
			value.Set(reflect.ValueOf(vals))

		case "Filter.Kinds":
			k := []int{generateKind()}
			value.Set(reflect.ValueOf(k))

		case "Filter.Tags":
			values := []string{}
			for range rand.Intn(3) {
				values = append(values, generateHexString())
			}
			m := model.TagMap{
				"e": values,
			}
			value.Set(reflect.ValueOf(m))

		case "Filter.Limit":
			l := int(rand.Int63n(100))
			value.Set(reflect.ValueOf(l))

		case "Filter.Until", "Filter.Since":
			ts := model.Timestamp(generateCreatedAt())
			value.Set(reflect.ValueOf(&ts))

		case "Expiration", "Videos", "Images", "Quotes", "References":
			value.Set(reflect.ValueOf(helperRandomBool(t)))

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
