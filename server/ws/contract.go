// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"sync"
)

type (
	Writer = adapters.WSWriter
	Config = config.Config
)

type (
	handler struct {
		subListenersMx sync.Mutex
		subListeners   map[adapters.WSWriter]map[string]*model.Subscription
	}
)

const applicationYamlKey = "cmd/subzero"
