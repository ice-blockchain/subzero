// SPDX-License-Identifier: ice License 1.0

package query

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"log"
	"strings"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type (
	dbClient struct {
		db                 *connector.DB
		relayPrivateKey    string
		relayURL           string
		rollbackableEvents *xsync.Map[string, *databaseRollbackRequest]
	}
)

var (
	//go:embed DDL.sql
	ddl string
)

func openDatabase(ctx context.Context, target string, runDDL bool, replicas ...string) *dbClient {
	client := &dbClient{
		rollbackableEvents: xsync.NewMap[string, *databaseRollbackRequest](),
	}
	options := []connector.Option{
		connector.WithMaster(target),
		connector.WithReplicas(replicas),
		connector.WithFieldNameMapper(func(in string) (out string) {
			n := strings.ToLower(in)
			switch n {
			case "createdat":
				out = "created_at"
			case "systemkind":
				out = "system_kind"
			case "referenceid":
				out = "reference_id"
			case "sigalg":
				out = "sig_alg"
			case "keyalg":
				out = "key_alg"
			case "masterpubkey":
				out = "master_pubkey"
			case "dtag":
				out = "d_tag"
			case "htag":
				out = "h_tag"
			case "hasimages":
				out = "has_images"
			case "hasvideos":
				out = "has_videos"
			case "addressvalue":
				out = "address"
			case "tagid":
				out = "tag_id"
			default:
				out = n
			}
			return out
		}),
	}

	if runDDL {
		options = append(options, connector.WithDDL(ddl))
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

func hashEvents(events ...*model.Event) (hash string) {
	var buf bytes.Buffer

	for _, e := range events {
		buf.WriteString(e.String())
	}
	sum := sha256.Sum256(buf.Bytes())

	return string(sum[:])
}
