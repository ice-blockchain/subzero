// SPDX-License-Identifier: ice License 1.0

package query

import (
	"bytes"
	"encoding/json"
	"html/template"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
			ts := generateCreatedAt()
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
				buildResult, err := db.generateSelectEventsSQL(t.Context(), filter)
				if err != nil {
					errCh <- errors.Errorf("failed to generate select events sql for set #%d (%#v): %w", i+1, set, err)
					return
				}

				sql := "EXPLAIN (FORMAT JSON, ANALYZE) " + buildResult.Statement
				result, err := connector.GetNamed[string](t.Context(), db.db, sql, buildResult.Params)
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
						errCh <- errors.Errorf("set #%d: found SCAN without INDEX; sql: %s; params: %#v", i+1, sql, buildResult.Params)
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

func helperCreateUsers(t *testing.T, db *dbClient, n int) []string {
	t.Helper()

	bar := progressbar.Default(int64(n), "creating users")
	defer bar.Finish()
	keys := make([]string, 0, n)
	for i := range n {
		bar.Add(1)

		key := model.GeneratePrivateKey()
		keys = append(keys, key)

		now := model.Timestamp(time.Now().UnixNano())
		metadata := model.ProfileMetadataContent{
			RegisteredAt: now,
			Name:         "User " + strconv.Itoa(i+1),
			About:        "This is user " + strconv.Itoa(i+1) + ".",
		}

		content, err := json.Marshal(metadata)
		require.NoError(t, err)

		var ev model.Event
		ev.Kind = nostr.KindProfileMetadata
		ev.CreatedAt = now + 1
		ev.Content = string(content)

		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))
	}

	return keys
}

func helperCreatePosts(t *testing.T, db *dbClient, keys []string, n int) []*model.Event {
	t.Helper()

	posts := make([]*model.Event, 0, n)
	bar := progressbar.Default(int64(n), "creating posts")
	for range n {
		bar.Add(1)

		hasImage := rand.Int64N(100) < 50
		hasVideo := rand.Int64N(100) < 50
		hasExpiration := rand.Int64N(100) < 50
		key := keys[rand.Int64N(int64(len(keys)))]

		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = model.Timestamp(time.Now().UnixNano())
		ev.Content = "This is a test post with some random content. " + generateRandomString(rand.IntN(100))

		if hasExpiration {
			ev.Tags = append(ev.Tags, model.Tag{"expiration", ev.CreatedAt.Add(time.Hour).String()})
		}

		if hasImage || hasVideo {
			imeta := model.Tag{"imeta"}
			if hasImage {
				imeta = append(imeta, "m image/jpeg")
			}
			if hasVideo {
				imeta = append(imeta, "m video/mp4")
			}
			ev.Tags = append(ev.Tags, imeta)
		}

		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		posts = append(posts, &ev)
	}
	bar.Finish()

	bar = progressbar.Default(int64(len(posts)), "inserting posts")
	batches := model.SplitBatch(posts, 100)
	for _, batch := range batches {
		bar.Add(len(batch))
		require.NoError(t, db.AcceptEvents(t.Context(), batch...))
	}
	bar.Finish()

	return posts
}

func helperCreatePostsReactions(t *testing.T, db *dbClient, keys []string, posts []*model.Event, n int) {
	t.Helper()

	const (
		reactionLike = iota
		reactionQuote
		reactionRepost
		reactionReply
		reactionMax
	)

	bar := progressbar.Default(int64(n), "creating reactions")
	reactions := make([]*model.Event, 0, n)
	for range n {
		bar.Add(1)

		key := keys[rand.Int64N(int64(len(keys)))]
		post := posts[rand.Int64N(int64(len(posts)))]
		reaction := rand.Int64N(reactionMax)

		var ev model.Event
		ev.CreatedAt = nostr.Now()
		switch reaction {
		case reactionLike:
			ev.Kind = nostr.KindReaction
			ev.Content = "+"
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"k", strconv.Itoa(int(post.Kind))},
				{"e", post.ID},
			}
		case reactionQuote:
			ev.Kind = nostr.KindTextNote
			ev.Content = "This is a quote reaction to a post. " + generateRandomString(rand.IntN(100))
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"q", post.ID},
			}
		case reactionRepost:
			ev.Kind = nostr.KindRepost
			ev.Content = post.String()
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"k", strconv.Itoa(int(post.Kind))},
				{"e", post.ID},
			}
		case reactionReply:
			ev.Kind = nostr.KindTextNote
			ev.Content = "This is a reply to a post. " + generateRandomString(rand.IntN(100))
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"e", post.ID, "", "root"},
				{"e", post.ID, "", "reply"},
			}
		default:
			t.Fatalf("unknown reaction type: %d", reaction)
		}

		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		reactions = append(reactions, &ev)
	}
	bar.Finish()

	bar = progressbar.Default(int64(len(reactions)), "inserting reactions")
	batches := model.SplitBatch(reactions, 100)
	for _, batch := range batches {
		bar.Add(len(batch))
		require.NoError(t, db.AcceptEvents(t.Context(), batch...))
	}
	bar.Finish()
}

func generateValidSearchCombinations(generic []string, flagged []string) [][]string {
	genericSubsets := combinations.All(generic)
	flaggedSubsets := generateFlagCombinations(flagged)

	totalCombinations := len(genericSubsets) * len(flaggedSubsets)
	allSubsets := make([][]string, 0, totalCombinations)

	for _, genSubset := range genericSubsets {
		for _, flagSubset := range flaggedSubsets {
			// Create a new slice with enough capacity.
			combined := make(
				[]string,
				0,
				len(genSubset)+len(flagSubset),
			)
			combined = append(combined, genSubset...)
			combined = append(combined, flagSubset...)
			allSubsets = append(allSubsets, combined)
		}
	}

	return allSubsets
}

func generateFlagCombinations(flagged []string) [][]string {
	flagMap := make(map[string][]string)
	var flagOrder []string
	for _, flag := range flagged {
		parts := strings.SplitN(flag, ":", 2)
		if len(parts) == 2 {
			key := parts[0]
			if _, exists := flagMap[key]; !exists {
				flagOrder = append(flagOrder, key)
			}
			flagMap[key] = append(flagMap[key], flag)
		}
	}
	return buildFlagSubsetsRecursive(flagOrder, flagMap)
}

func buildFlagSubsetsRecursive(order []string, flagMap map[string][]string) (result [][]string) {
	if len(order) == 0 {
		return [][]string{{}}
	}

	currentFlagKey := order[0]
	remainingOrder := order[1:]
	subCombinations := buildFlagSubsetsRecursive(remainingOrder, flagMap)

	result = append(result, subCombinations...)
	for _, flagVariant := range flagMap[currentFlagKey] {
		for _, sub := range subCombinations {
			newCombination := make([]string, 0, len(sub)+1)
			newCombination = append(newCombination, sub...)
			newCombination = append(newCombination, flagVariant)
			result = append(result, newCombination)
		}
	}
	return result
}

func TestQueryFuzzDependencies(t *testing.T) {
	t.Parallel()

	// The test is very resource-intensive, and may eat up to 30GB of RAM.
	if os.Getenv("TEST_FUZZ_DEPS") != "true" {
		t.Skipf("set TEST_FUZZ_DEPS=true to run this test")
	}

	var (
		generic = []string{
			"kind30008+profile_badges>kind30009>kind8",
			"kind1>kind30008+profile_badges>kind30009>kind8",
			"kind6>kind10002",
			"kind1+q>kind10002",
			"kind1>kind0",
			"kind1>{{.Pubkey}}@kind1+e+root",
			"kind1>{{.Pubkey}}@kind1+q",
			"kind1>{{.Pubkey}}@kind6",
			"kind1>{{.Pubkey}}@kind7",
			"kind3>kind0",
			"kind1>kind6400+kind1+group+reply",
			"kind1>kind6400+kind6+group+e",
			"kind1>kind6400+kind1+group+q",
			"kind1>kind6400+kind7+group+content",
			"kind1>kind6400+kind1754+group+content",
			"kind0>kind6400+kind3+group+p",
		}
		extensions = []string{
			"media:true", "media:false",
			"quotes:true", "quotes:false",
			"references:true", "references:false",
			"videos:true", "videos:false",
			"images:true", "images:false",
			"expiration:true", "expiration:false",
		}
	)

	type testReselt struct {
		Filter  *model.Filter
		Err     error
		Explain string
	}

	db := helperNewDatabase(t)
	defer db.Close()

	keys := helperCreateUsers(t, db, 1_000)
	currentKey := keys[rand.Int64N(int64(len(keys)))]

	for i := range generic {
		var buf bytes.Buffer

		buf.WriteString(`include:dependencies:`)
		tpl := template.Must(template.New(strconv.Itoa(i)).Parse(generic[i]))
		pub, err := model.GetPublicKey(currentKey)
		require.NoError(t, err)
		tpl.Execute(&buf, map[string]string{
			"Pubkey": pub,
		})
		generic[i] = buf.String()
		t.Logf("generic[%d]: %s", i, generic[i])
	}

	deps := generateValidSearchCombinations(generic, extensions)
	t.Logf("found %d total dependency combination(s)", len(deps)) // ~10G of RAM, ~47_775_015 elements.
	posts := helperCreatePosts(t, db, keys, 100_0)
	helperCreatePostsReactions(t, db, keys, posts, 100_0)

	w := min(runtime.NumCPU()*3, 100)

	t.Run("Fuzz", func(t *testing.T) {
		t.Logf("testing %d sets with %d workers", len(deps), w)
		bar := progressbar.Default(int64(len(deps)), "testing sets")
		pool := pond.NewPool(w)
		errCh := make(chan testReselt, w)

		for i, set := range deps {
			i, set := i, set
			pool.Submit(func() {
				defer bar.Add(1)
				filter := model.Filter{
					Kinds:  []int{nostr.KindTextNote, nostr.KindRepost},
					Search: strings.Join(set, " "),
				}
				sql, params, err := db.generateSelectEventsSQL(t.Context(), filter)
				if err != nil {
					errCh <- testReselt{Err: errors.Errorf("failed to generate select events sql for set #%d (%#v): %w", i+1, set, err)}
					return
				}

				sql = "EXPLAIN (FORMAT JSON, ANALYZE) " + sql
				result, err := connector.GetNamed[string](t.Context(), db.db, sql, params)
				if err != nil {
					errCh <- testReselt{Err: errors.Errorf("failed to execute query for set #%d: %w", i+1, err)}
					return
				}
				if result == nil {
					errCh <- testReselt{Err: errors.Errorf("nil result for set #%d", i+1)}
					return
				}

				var q []Query
				err = json.Unmarshal([]byte(*result), &q)
				if err != nil {
					errCh <- testReselt{Err: errors.Errorf("failed to unmarshal query result for set #%d: %w", i+1, err)}
					return
				}

				if helperQueryHas(t, q, "Seq Scan") {
					var emptyFilter model.Filter
					if !nostr.FilterEqual(filter, emptyFilter) {
						errCh <- testReselt{
							Filter:  &filter,
							Err:     errors.Errorf("set #%d: found SCAN without INDEX; sql: %s; params: %#v", i+1, sql, params),
							Explain: *result,
						}
					}
				}
			})
		}

		go func() {
			pool.StopAndWait()
			t.Logf("finished testing %d sets", len(deps))
			close(errCh)
		}()

		for err := range errCh {
			t.Errorf("error: %v", err.Err)
			if err.Explain != "" {
				t.Errorf("explain: %s", err.Explain)
			}
			if err.Filter != nil {
				t.Errorf("filter: %s", err.Filter.String())
			}
			t.FailNow() // Fail fast on the first error.
		}
	})
}
