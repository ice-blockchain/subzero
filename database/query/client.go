// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query/ddl"
	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type (
	dbOperationReporter func(context.Context, operationType, error)
	dbClient            struct {
		db                 *connector.DB
		operationReporter  dbOperationReporter
		rollbackableEvents *xsync.Map[eventHash, *databaseRollbackRequest]
		relayPrivateKey    string
		relayURL           string
		hasReadURLs        bool
	}

	eventHash [sha256.Size]byte
)

var (
	databaseEventFieldMap = map[string]string{
		"createdat":               "created_at",
		"referenceid":             "reference_id",
		"sigalg":                  "sig_alg",
		"keyalg":                  "key_alg",
		"masterpubkey":            "master_pubkey",
		"dtag":                    "d_tag",
		"htag":                    "h_tag",
		"hasimages":               "has_images",
		"hasvideos":               "has_videos",
		"addressvalue":            "address",
		"tagid":                   "tag_id",
		"systemid":                "system_id",
		"lookupcreatedat":         "lookup_created_at",
		"hasreferences":           "has_references",
		"hasephemeralattestation": "has_ephemeral_attestation",
	}
)

func mapStructEntryToDatabaseFieldName(in string) string {
	n := strings.ToLower(in)
	if mapped, ok := databaseEventFieldMap[n]; ok {
		return mapped
	}
	if mapped, ok := pnFieldMap[n]; ok {
		return mapped
	}
	return n
}

func openDatabase(ctx context.Context, writeURLs []string, readURLs []string, runDDL bool, ext ...connector.Option) *dbClient {
	client := &dbClient{
		rollbackableEvents: xsync.NewMap[eventHash, *databaseRollbackRequest](),
		hasReadURLs:        len(readURLs) > 0,
		operationReporter: func(context.Context, operationType, error) {
		},
	}
	options := []connector.Option{
		connector.WithFieldNameMapper(mapStructEntryToDatabaseFieldName),
	}
	if len(writeURLs) > 0 {
		options = append(options, connector.WithWriteURLs(writeURLs...))
	}
	if len(readURLs) > 0 {
		options = append(options, connector.WithReadURLs(readURLs...))
	}
	if runDDL {
		options = append(options, connector.WithDDL(&ddl.Files))
	}
	options = append(options, ext...)

	db, err := connector.New(ctx, options...)
	if err != nil {
		log.Panic().Err(err).Msg("failed to open database")
	}
	client.db = db

	return client
}

func (client *dbClient) WithOperationReporter(reporter dbOperationReporter) *dbClient {
	client.operationReporter = reporter
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
