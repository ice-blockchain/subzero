// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/cometbft/multiplex/client"
)

type errClient struct{}
type callbackClient struct {
	broadcastTxCallback      func(userAddress string, relays []string, transactions ...client.Transaction)
	broadcastRemovalCallback func(userAddress string, relays []string, transactions ...client.Transaction)
}

func (c *callbackClient) BroadcastTx(ctx context.Context, userAddress string, relays []string, notifier chan<- client.BroadcastStatus, transactions ...client.Transaction) {
	c.broadcastTxCallback(userAddress, relays, transactions...)
	notifier <- client.BroadcastStatus{Error: nil, TxHashes: nil}
}

func (c *callbackClient) BroadcastTxRemoval(ctx context.Context, userAddress string, relays []string, notifier chan<- client.BroadcastStatus, transactions ...client.Transaction) {
	c.broadcastRemovalCallback(userAddress, relays, transactions...)
	notifier <- client.BroadcastStatus{Error: nil, TxHashes: nil}
}

func (c *callbackClient) GetAcceptor() client.Acceptor {
	log.Panic().Err(errors.New("should not be called"))
	return nil
}
func (e *errClient) GetAcceptor() client.Acceptor {
	log.Panic().Err(errors.New("should not be called"))
	return nil
}

func (e *errClient) BroadcastTx(ctx context.Context, userAddress string, relays []string, notifier chan<- client.BroadcastStatus, transactions ...client.Transaction) {
	notifier <- client.BroadcastStatus{
		Error:    errors.New("error"),
		TxHashes: nil,
	}
}

func (e *errClient) BroadcastTxRemoval(ctx context.Context, userAddress string, relays []string, notifier chan<- client.BroadcastStatus, transactions ...client.Transaction) {
	notifier <- client.BroadcastStatus{
		Error:    errors.New("error from errClient"),
		TxHashes: nil,
	}
}

func NewErrornousClient() client.Client {
	return &errClient{}
}
func NewCallbackClient(tx, removal func(userAddress string, relays []string, transactions ...client.Transaction)) client.Client {
	return &callbackClient{
		broadcastTxCallback:      tx,
		broadcastRemovalCallback: removal,
	}
}
