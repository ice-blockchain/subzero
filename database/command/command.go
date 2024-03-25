package command

import "github.com/ice-blockchain/subzero/model"

var syncQuery func(*model.Event) error

func RegisterQuerySyncer(sync func(*model.Event) error) {
	syncQuery = sync
}
