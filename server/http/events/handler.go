// SPDX-License-Identifier: ice License 1.0

package events

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func GetEventByAddress(ctx *gin.Context) {
	addressStr := ctx.Param("eventAddress")

	if addressStr == "" {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "event address is required"})
		return
	}

	it := query.GetStoredEvents(ctx, model.Filter{
		Addresses: []string{addressStr},
		Limit:     1,
	})

	var event *model.Event
	for ev, err := range it {
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		event = ev
		break
	}

	if event == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.JSON(http.StatusOK, event)
}
