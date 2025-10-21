// SPDX-License-Identifier: ice License 1.0

//go:build test

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"math/rand/v2"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/panjf2000/ants/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type Plan struct {
	Plans       []Plan   `json:"Plans"`
	NodeType    string   `json:"Node Type"`
	IndexName   string   `json:"Index Name"`
	IndexCond   string   `json:"Index Cond"`
	SortKey     []string `json:"Sort Key"`
	Filter      string   `json:"Filter"`
	RowsRemoved int64    `json:"Rows Removed by Filter"`
}

type Query struct {
	Plan     Plan    `json:"Plan"`
	ExecTime float64 `json:"Execution Time"`
	Rows     uint64  `json:"-"`
}

type QueryResult struct {
	Filter    *model.Filter
	Err       error
	Explain   string
	SeqFilter string
	Position  int
}

func (p *Plan) Has(op string) (bool, string) {
	if p.NodeType == op {
		return true, p.Filter
	}
	for i := range p.Plans {
		if ok, f := p.Plans[i].Has(op); ok {
			return ok, f
		}
	}
	return false, ""
}

func queryHas(q []Query, op string) (bool, string) {
	for _, subq := range q {
		if ok, f := subq.Plan.Has(op); ok {
			return ok, f
		}
	}
	return false, ""
}

var (
	emptyFilter model.Filter

	allSearchDependencies = []string{
		"kind30008+profile_badges>kind30009>kind8",
		"kind30175>kind30008+profile_badges>kind30009>kind8",
		"kind16>kind10002",
		"kind30175+q>kind10002",
		"kind30175>kind0",
		"kind30175>{{.Pubkey}}@kind30175+e+root",
		"kind30175>{{.Pubkey}}@kind30175+q",
		"kind30175>{{.Pubkey}}@kind16",
		"kind30175>{{.Pubkey}}@kind7",
		"kind30175>kind0",
		"kind30175>kind6400+kind30175+group+reply",
		"kind30175>kind6400+kind6+group+e",
		"kind30175>kind6400+kind30175+group+q",
		"kind30175>kind6400+kind7+group+content",
		"kind30175>kind6400+kind1754+group+content",
		"kind0>kind6400+kind3+group+p",
		"kind0>kind6400+kind30175+expiration",
	}
	allSearchExtensions = []string{
		"media:true", "media:false",
		"quotes:true", "quotes:false",
		"references:true", "references:false",
		"videos:true", "videos:false",
		"images:true", "images:false",
		"expiration:true", "expiration:false",
	}
	// allSearchExtensions + allSearchDependencies = 95,550,759 combinations.
)

func newContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		force := false
		for sig := range c {
			if force {
				log.Warn().Str("signal", sig.String()).Msg("forceful shutdown")
				os.Exit(2)
			} else {
				log.Warn().Str("signal", sig.String()).Msg("graceful shutdown")
				cancel()
				force = true
			}
		}
	}()

	return ctx
}

func executeFilter(ctx context.Context, analyze bool, filter model.Filter) ([]Query, string, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	if analyze {
		sql, params, err := query.GenerateSelectEventsSQL(ctx, filter)
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to generate select events sql")
		}

		sql = "EXPLAIN (FORMAT JSON, COSTS OFF, BUFFERS OFF) " + sql
		result, err := connector.GetNamed[string](ctx, query.Database(), sql, params)
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to execute query")
		}

		if result == nil {
			return nil, "", errors.New("nil result")
		}

		var q []Query
		err = json.Unmarshal([]byte(*result), &q)
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to unmarshal query result")
		}
		return q, *result, nil
	}

	var totalEvents uint64
	it := query.GetStoredEvents(ctx, filter)
	for _, err := range it {
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to execute query")
		}
		totalEvents++
	}

	return []Query{{Rows: totalEvents}}, "", nil
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339Nano,
	})

	databaseURL := flag.String("database", "postgres://subzero:subzero@localhost/subzero?sslmode=disable", "Database URL")
	threadCount := flag.Int("threads", runtime.NumCPU()*2, "Number of threads to use")
	postsCount := flag.Int("posts", 1000, "Number of posts to create")
	usersCount := flag.Int("users", 1000, "Number of users to create")
	reactionsCount := flag.Int("reactions", 1000, "Number of reactions to create")
	skipDataGen := flag.Bool("skip-data-gen", false, "Skip data generation")
	positionToUse := flag.Int("position", 0, "Position to use")
	withFilter := flag.String("with-filter", "", "Test given filter only")
	limit := flag.Int("limit", 0, "Limit number of combinations to execute (0 means no limit)")
	doAnalyze := flag.Bool("analyze", false, "Use EXPLAIN ANALYZE instead of SELECT")
	runDDL := flag.Bool("run-ddl", false, "Run DDL migrations")

	flag.Parse()

	if *databaseURL == "" {
		log.Fatal().Msg("database URL is required")
	} else if *threadCount <= 0 {
		*threadCount = 1
	}

	ctx, cancel := appcontext.NewAppContext(newContext())
	defer cancel()

	log.Info().Str("url", *databaseURL).Msg("database initialization ...")
	query.MustInit(ctx, query.WithConfig(&query.Config{
		PrivateKey:               model.GeneratePrivateKey(),
		RelayURL:                 "wss://example.com",
		DisableSelfTest:          true,
		PeriodicSelfTestInterval: -1,
		WriteURLs:                []string{*databaseURL},
		RunDDL:                   *runDDL,
	}))
	log.Printf("database ready")

	total, err := query.CountEvents(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to count events")
	}
	log.Info().Msgf("database has %d events", total)

	if *withFilter != "" {
		var filter model.Filter

		err := filter.UnmarshalJSON([]byte(*withFilter))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to unmarshal filter")
		}
		log.Info().Msgf("testing with filter: %s", filter.String())

		result, explain, err := executeFilter(ctx, *doAnalyze, filter)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to execute filter")
		}

		if ok, f := queryHas(result, "Seq Scan"); ok && !strings.Contains(f, "SubPlan") {
			if !nostr.FilterEqual(filter, emptyFilter) {
				log.Warn().Msg("error: found SCAN without INDEX")
				log.Warn().Msgf("  filter: %s", filter.String())
				log.Warn().Msgf("  explain: %s", explain)
				log.Warn().Msgf("  seq: %s", f)
				os.Exit(1)
			}
		}
		return
	}

	var currentPublicKey string
	if !*skipDataGen {
		var err error
		log.Info().
			Int("users", *usersCount).
			Int("posts", *postsCount).
			Int("reactions", *reactionsCount).
			Msg("creating dummy data ...")
		keys := createUsers(ctx, *usersCount)
		posts := createPosts(ctx, keys, *postsCount)
		createPostsReactions(ctx, keys, posts, *reactionsCount)
		log.Info().Msg("dummy data created")
		currentPublicKey, err = model.GetPublicKey(keys[rand.Int64N(int64(len(keys)))])
		if err != nil {
			log.Fatal().Err(err).Msg("failed to get public key")
		}
	} else {
		log.Info().Msg("skipping data generation")
		var pubkeys []string
		it := query.GetStoredEvents(ctx, model.Filter{Kinds: []int{nostr.KindProfileMetadata}, Limit: 400})
		for ev, err := range it {
			if err != nil {
				log.Fatal().Err(err).Msg("failed to get stored events")
			}
			pubkeys = append(pubkeys, ev.GetMasterPublicKey())
		}
		if len(pubkeys) == 0 {
			log.Fatal().Msg("no users found in the database, cannot proceed")
		}
		currentPublicKey = pubkeys[rand.Int64N(int64(len(pubkeys)))]
	}
	log.Info().Msgf("using public key: %s", currentPublicKey)

	if *doAnalyze {
		log.Warn().Msg("using EXPLAIN ANALYZE")
	} else {
		log.Warn().Msg("using simple SELECT")
	}

	log.Info().Msg("preparing combinations ...")
	for i := range allSearchDependencies {
		var buf bytes.Buffer

		buf.WriteString(`include:dependencies:`)
		tpl := template.Must(template.New(strconv.Itoa(i)).Parse(allSearchDependencies[i]))
		tpl.Execute(&buf, map[string]string{
			"Pubkey": currentPublicKey,
		})
		allSearchDependencies[i] = buf.String()
		log.Info().Msgf("search dependency %d: %s", i, allSearchDependencies[i])
	}

	iterator, err := NewSearchCombinationsIterator(allSearchDependencies, allSearchExtensions, *positionToUse)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create combinations iterator")
	}

	total = int64(iterator.Total() - *positionToUse)
	if *limit > 0 && *limit < int(total) {
		total = int64(*limit)
	}
	log.Info().Msgf("total combinations to execute: %d", total)

	debug.SetGCPercent(20)
	bar := progressbar.Default(int64(total), "testing SELECT combinations")
	pool, err := ants.NewPool(*threadCount,
		ants.WithNonblocking(false),
		ants.WithMaxBlockingTasks(*threadCount*2),
	)
	panicOnErr(err)
	errCh := make(chan QueryResult, *threadCount+1)

	go func() {
		for err := range errCh {
			log.Error().Err(err.Err).Msg("query execution error")
			if err.Explain != "" {
				log.Info().Msgf("  explain: %s", err.Explain)
			}
			if err.Filter != nil {
				log.Info().Msgf("  filter: %s", err.Filter.String())
			}
			if err.SeqFilter != "" {
				log.Info().Msgf("  seq: %s", err.SeqFilter)
			}
			log.Fatal().Int("position", err.Position).Msg("stopping due to error")
		}
	}()

	var count int64
	var eventsFetched uint64
	for ctx.Err() == nil {
		combination, hasNext := iterator.Next()
		if !hasNext {
			break
		}

		i := iterator.Position() - 1

		for ctx.Err() == nil {
			err = pool.Submit(func() {
				defer bar.Add(1)

				filter := model.Filter{
					Kinds:  []int{model.CustomIONKindEditableTextNote, nostr.KindArticle, nostr.KindGenericRepost},
					Search: strings.Join(combination, " "),
				}

				result, explain, err := executeFilter(ctx, *doAnalyze, filter)
				if err != nil {
					errCh <- QueryResult{
						Filter:   &filter,
						Err:      errors.Wrapf(err, "set #%d: failed to execute filter", i),
						Position: i,
					}
					return
				}

				if !*doAnalyze && len(result) > 0 {
					eventsFetched += result[0].Rows
				}

				if ok, f := queryHas(result, "Seq Scan"); ok && !strings.Contains(f, "SubPlan") {
					if !nostr.FilterEqual(filter, emptyFilter) {
						errCh <- QueryResult{
							Filter:    &filter,
							Err:       errors.Errorf("set #%d: found SCAN without INDEX", i),
							SeqFilter: f,
							Explain:   explain,
							Position:  i,
						}
					}
				}
			})
			if err == nil {
				// Submitted successfully.
				break
			} else if errors.Is(err, ants.ErrPoolOverload) {
				// Pool is overloaded, wait and retry.
				time.Sleep(100 * time.Millisecond)
			} else {
				// Some other error.
				log.Fatal().Err(err).Msg("failed to submit task to pool")
			}
		}

		count++
		if *limit > 0 && count >= int64(*limit) {
			log.Printf("reached limit of %d, stopping", *limit)
			break
		}
		// Force GC periodically.
		if i%10_000 == 0 {
			log.Debug().Msg("running GC ...")
			debug.FreeOSMemory()
		}
		if i%20_000 == 0 {
			log.Debug().Msg("database reset ...")
			query.Database().Reset()
		}
		if i%100_000 == 0 && !*doAnalyze && i > 0 {
			log.Info().Msgf("executed %d combinations, fetched %d events so far", i, eventsFetched)
		}
	}

	bar.Finish()
	pool.Release()
	close(errCh)

	log.Info().Msgf("all done, executed %d combinations", count)
	log.Info().Msgf("next position: %d", iterator.Position())
	if !*doAnalyze {
		log.Info().Msgf("total events fetched: %d", eventsFetched)
	}
}
