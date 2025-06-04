// SPDX-License-Identifier: ice License 1.0

package query

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
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
		rollbackableEvents *xsync.Map[string, *databaseRollbackRequest]
	}
)

var (
	//go:embed ddl/*.sql
	ddl embed.FS
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
			case "lookupcreatedat":
				out = "lookup_created_at"
			case "hasreferences":
				out = "has_references"
			default:
				out = n
			}
			return out
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

func hashEvents(events ...*model.Event) (hash string) {
	var buf bytes.Buffer

	for _, e := range events {
		buf.WriteString(e.String())
	}
	sum := sha256.Sum256(buf.Bytes())

	return string(sum[:])
}
