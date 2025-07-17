// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"log"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type (
	dbClient struct {
		db                 *connector.DB
		relayPrivateKey    string
		relayURL           string
		rollbackableEvents *xsync.Map[eventHash, *databaseRollbackRequest]
	}

	eventHash [sha256.Size]byte
)

var (
	//go:embed ddl/*.sql
	ddl embed.FS

	databaseEventFieldMap = map[string]string{
		"createdat":       "created_at",
		"referenceid":     "reference_id",
		"sigalg":          "sig_alg",
		"keyalg":          "key_alg",
		"masterpubkey":    "master_pubkey",
		"dtag":            "d_tag",
		"htag":            "h_tag",
		"hasimages":       "has_images",
		"hasvideos":       "has_videos",
		"addressvalue":    "address",
		"tagid":           "tag_id",
		"lookupcreatedat": "lookup_created_at",
		"hasreferences":   "has_references",
	}
)

func readDDL() string {
	var sb strings.Builder

	files, err := ddl.ReadDir("ddl")
	if err != nil {
		log.Panicf("failed to read DDL directory: %v", err)
	}

	var names []string
	for _, file := range files {
		names = append(names, file.Name())
	}

	extractPrefix := func(name string) (value int) {
		for _, c := range name {
			if unicode.IsDigit(c) {
				value = value*10 + int(c-'0')
			} else {
				break
			}
		}
		return value
	}

	// Sort files by their numeric prefix.
	sort.SliceStable(names, func(i, j int) bool {
		return extractPrefix(names[i]) < extractPrefix(names[j])
	})

	for i, fileName := range names {
		target := path.Join("ddl", fileName)
		content, err := ddl.ReadFile(target)
		if err != nil {
			log.Panicf("failed to read DDL file %s: %v", target, err)
		}
		if i > 0 {
			sb.WriteString("--------")
		}
		sb.WriteString(string(content))
		sb.WriteRune('\n')
	}

	return sb.String()
}

func openDatabase(ctx context.Context, target string, runDDL bool, replicas ...string) *dbClient {
	client := &dbClient{
		rollbackableEvents: xsync.NewMap[eventHash, *databaseRollbackRequest](),
	}
	options := []connector.Option{
		connector.WithMaster(target),
		connector.WithReplicas(replicas),
		connector.WithFieldNameMapper(func(in string) string {
			n := strings.ToLower(in)
			if mapped, ok := databaseEventFieldMap[n]; ok {
				return mapped
			}
			return n
		}),
	}

	if runDDL {
		options = append(options, connector.WithDDL(readDDL()))
	}

	db, err := connector.New(ctx, options...)
	if err != nil {
		log.Panicf("failed to open database: %v", err)
	}
	client.db = db

	return client
}

func (client *dbClient) Close() (err error) {
	if client.db != nil {
		err = errors.Join(err, client.db.Close())
	}
	return err
}

func (client *dbClient) WithRelayURL(relayURL string) *dbClient {
	client.relayURL = relayURL

	return client
}

func (client *dbClient) WithPrivateKey(privateKey string) *dbClient {
	if privateKey == "" {
		panic("private key is empty")
	}
	client.relayPrivateKey = privateKey

	return client
}

func hashEvents(events ...*model.Event) (sum eventHash) {
	h := sha256.New()

	for _, e := range events {
		h.Write(e.Serialize())
	}

	h.Sum(sum[:0])

	return sum
}

func (h eventHash) String() string {
	return hex.EncodeToString(h[:])
}
