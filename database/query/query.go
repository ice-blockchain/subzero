package query

import (
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/model"
)

func init() {
	command.RegisterQuerySyncer(acceptEvent)
}

func acceptEvent(event *model.Event) error {
	return nil
}
