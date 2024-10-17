// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/errgroup"

	"github.com/ice-blockchain/subzero/model"
)

type (
	JobFeedbackStatus  = string
	DvmServiceProvider interface {
		AcceptJob(ctx context.Context, event *model.Event) error
	}
	JobItem interface {
		Process(ctx context.Context, e *model.Event) (payload string, err error)
		RequiredPaymentAmount() float64
		IsBidAmountEnough(amount string) bool
		OnPaymentRequiredFeedback(ctx context.Context, event *model.Event) error
		OnProcessingFeedback(ctx context.Context, event *model.Event) error
		OnSuccessFeedback(ctx context.Context, event *model.Event) error
		OnErrorFeedback(ctx context.Context, event *model.Event, inErr error) error
		OnPartialFeedback(ctx context.Context, event *model.Event) error
	}
	dvmProcessCancelInfo struct {
		RelayList  []*nostr.Relay
		CancelFunc context.CancelFunc
	}
	dvm struct {
		serviceProviderPrivKey string
		serviceProviderPubkey  string
		minLeadingZeroBits     int
		dvmProcessCancel       sync.Map
	}
)

var (
	jobTimeoutDeadline = 1 * time.Minute
)

func NewDvms(minLeadingZeroBits int, pubKey, privKey string) DvmServiceProvider {
	return &dvm{
		serviceProviderPubkey:  pubKey,
		serviceProviderPrivKey: privKey,
		minLeadingZeroBits:     minLeadingZeroBits,
		dvmProcessCancel:       sync.Map{},
	}
}

func (d *dvm) AcceptJob(ctx context.Context, event *model.Event) error {
	if (event.Kind < 5000 && event.Kind != nostr.KindDeletion) || event.Kind > 5999 {
		return nil
	}
	if err := event.Validate(); err != nil {
		return errorx.Errorf("wrong dvm job: %v", event)
	}
	if event.Kind == nostr.KindDeletion {
		if err := d.handleDeletionEvent(ctx, event); err != nil {
			return errors.Wrapf(err, "can't handle deletion event: %v", err)
		}

		return nil
	}
	if false && !d.isServiceProviderCustomerInterestedIn(event) {
		log.Printf("dvm job is not for specified for this service provider: %v", event)

		return nil
	}
	go d.process(event)

	return nil
}

func (d *dvm) isServiceProviderCustomerInterestedIn(event *model.Event) bool {
	for _, tag := range event.Tags {
		if tag.Key() == "p" {
			return tag.Value() == d.serviceProviderPubkey
		}
	}

	return false
}

func (d *dvm) handleDeletionEvent(ctx context.Context, event *model.Event) error {
	if event.Kind == nostr.KindDeletion {
		stopEventID := event.Tags.GetFirst([]string{"e"}).Value()
		stopEventKind, err := strconv.Atoi(event.Tags.GetFirst([]string{"k"}).Value())
		if err != nil {
			return errors.Wrapf(err, "can't parse stop event kind: %v", err)
		}
		if stopEventKind < 5000 || stopEventKind > 5999 {
			return nil
		}
		reqCtx, reqCancel := context.WithTimeout(ctx, jobTimeoutDeadline)
		defer reqCancel()
		if err := d.stopEvent(reqCtx, event, stopEventID); err != nil {
			return errors.Wrapf(err, "dvm job deletion kind, failed to stop event with id %q and kind %d: %v", stopEventID, stopEventKind, err)
		}
	}

	return nil
}

func (d *dvm) process(event *model.Event) {
	reqCtx, reqCancel := context.WithTimeout(context.Background(), jobTimeoutDeadline)
	defer func() {
		reqCancel()
		d.dvmProcessCancel.Delete(event.GetID())
	}()
	var relayList []string
	for _, tag := range event.Tags {
		if tag.Key() == "param" && tag.Value() == "relay" {
			relayList = append(relayList, tag[2])
		}
	}
	outputRelays, err := connectToRelays(reqCtx, relayList)
	if err != nil {
		log.Printf("failed to connect to job relays to publish response: %v, err: %v", event, err)

		return
	}
	defer closeRelays(outputRelays)
	d.dvmProcessCancel.Store(event.GetID(), &dvmProcessCancelInfo{
		RelayList:  outputRelays,
		CancelFunc: reqCancel,
	})
	var (
		payload string
		job     JobItem
	)
	switch event.Kind {
	case model.KindJobNostrEventCount:
		job = newNostrEventCountJob(outputRelays, d.serviceProviderPubkey, d.serviceProviderPrivKey, d.minLeadingZeroBits)
	default:
		log.Printf("dvm kind:%v not supported", event.Kind)
	}
	if bidTag := event.Tags.GetFirst([]string{"bid"}); bidTag != nil && !job.IsBidAmountEnough(bidTag.Value()) {
		if err := job.OnPaymentRequiredFeedback(reqCtx, event); err != nil {
			log.Printf("failed to publish job error feedback: %v", event)
		}

		return
	}
	payload, err = job.Process(reqCtx, event)
	if err != nil {
		if fErr := job.OnErrorFeedback(reqCtx, event, err); fErr != nil {
			log.Printf("failed to publish job error feedback: %v", fErr)
		}
		log.Printf("failed to process job: %v, err:%v", event, err)

		return
	}
	if err := d.publishJobResult(reqCtx, event, outputRelays, payload, job.RequiredPaymentAmount()); err != nil {
		log.Printf("failed to publish job result: %v", err)
		if fErr := job.OnErrorFeedback(reqCtx, event, err); fErr != nil {
			log.Printf("failed to publish job error feedback: %v", fErr)
		}
	}
}

func (d *dvm) publishJobResult(ctx context.Context, incomingEvent *model.Event, relays []*nostr.Relay, payload string, reqiredPaymentAmount float64) error {
	request, err := incomingEvent.Strinfify()
	if err != nil {
		return errors.Wrap(err, "failed to stringify event")
	}
	result := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   payload,
			PubKey:    d.serviceProviderPubkey,
			Kind:      incomingEvent.Kind + 1000,
			Tags: nostr.Tags{
				[]string{"request", request},
				[]string{"e", incomingEvent.GetID()},
				[]string{"i", incomingEvent.Content},
				[]string{"p", incomingEvent.PubKey},
			},
		},
	}
	if reqiredPaymentAmount > 0 {
		result.Tags = append(result.Tags, nostr.Tag{"amount", fmt.Sprint(reqiredPaymentAmount)})
	}
	if err := signWithLeadingZeroBits(ctx, &result, d.serviceProviderPrivKey, d.minLeadingZeroBits); err != nil {
		return errors.Wrap(err, "failed to sign event")
	}
	eg := errgroup.Group{}
	for _, relay := range relays {
		eg.Go(func() error {
			return errors.Wrapf(relay.Publish(ctx, result.Event), "failed to publish job result to relay: %v", relay.URL)
		})
	}

	return errors.Wrapf(eg.Wait(), "can't publish to some relay job result for: %v", incomingEvent)
}

func (d *dvm) stopEvent(ctx context.Context, event *model.Event, stopJobID string) error {
	cancelInfo, ok := d.dvmProcessCancel.LoadAndDelete(event.GetID())
	if !ok {
		return nil
	}

	return errors.Wrapf(publishJobFeedback(
		ctx,
		event,
		model.JobFeedbackStatusError,
		cancelInfo.(*dvmProcessCancelInfo).RelayList,
		d.serviceProviderPubkey,
		d.serviceProviderPrivKey,
		fmt.Sprintf("Job %s has been stopped", stopJobID),
		0,
		d.minLeadingZeroBits,
	), "can't publish error feedback: %v", event)
}

func publishJobFeedback(ctx context.Context, incomingEvent *model.Event, status JobFeedbackStatus, relays []*nostr.Relay, serviceProviderPubKey, serviceProviderPrivateKey, payload string, reqiredPaymentAmount float64, minLeadingZeroBits int) error {
	result := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    serviceProviderPubKey,
			Content:   payload,
			Kind:      nostr.KindJobFeedback,
			Tags: nostr.Tags{
				nostr.Tag{"status", status},
				nostr.Tag{"e", incomingEvent.GetID()},
				nostr.Tag{"p", incomingEvent.PubKey},
			},
		},
	}
	if reqiredPaymentAmount > 0 {
		result.Tags = append(result.Tags, nostr.Tag{"amount", fmt.Sprint(reqiredPaymentAmount)})
	}
	if err := signWithLeadingZeroBits(ctx, &result, serviceProviderPrivateKey, minLeadingZeroBits); err != nil {
		return errors.Wrap(err, "failed to sign event")
	}
	eg := errgroup.Group{}
	for _, relay := range relays {
		relayUrl := relay.URL
		eg.Go(func() error {
			return errors.Wrapf(relay.Publish(ctx, result.Event), "failed to publish job feedback to relay: %v", relayUrl)
		})
	}

	return errors.Wrapf(eg.Wait(), "can't publish some of job feedback: %v", incomingEvent)
}

func connectToRelays(ctx context.Context, relayList []string) (resultRelays []*nostr.Relay, err error) {
	resultRelays = make([]*nostr.Relay, 0, len(relayList))
	for _, relayUrl := range relayList {
		for ix := 0; ix < len(relayList); ix++ {
			relay, err := connectToRelay(ctx, relayUrl)
			if err != nil {
				log.Printf("ERROR: failed to connect to relay: %v, err: %v", relayUrl, err)

				continue
			}
			resultRelays = append(resultRelays, relay)
		}
		log.Printf("Established %v conns to %v", len(resultRelays), relayUrl)
	}

	return resultRelays, nil
}

func connectToRelay(ctx context.Context, url string) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url)
	err := relay.Connect(ctx)
	if err != nil {
		return nil, err
	}

	return relay, nil
}

func closeRelays(relayList []*nostr.Relay) {
	for _, r := range relayList {
		if err := r.Close(); err != nil {
			log.Printf("Can't close relay:%v, err:%v", r.URL, err)

			continue
		}
		log.Printf("Closed relay:%v", r.URL)
	}
}

func deduplicateEvents(events []*nostr.Event) []*nostr.Event {
	result := make([]*nostr.Event, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if _, ok := seen[event.GetID()]; ok {
			continue
		}
		result = append(result, event)
		seen[event.GetID()] = struct{}{}
	}

	return result
}

func signWithLeadingZeroBits(ctx context.Context, event *model.Event, privateKey string, minLeadingZeroBits int) error {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := event.Sign(privateKey); err != nil {
		return errors.Wrapf(err, "can't sign event: %v", event)
	}
	if minLeadingZeroBits > 0 {
		if err := event.GenerateNIP13(reqCtx, minLeadingZeroBits); err != nil {
			return errors.Wrapf(err, "can't generate nip 13 with min leading zero bits:%v", minLeadingZeroBits)
		}
		if err := event.Sign(privateKey); err != nil {
			return errors.Wrapf(err, "can't sign event: %v", event)
		}
	}

	return nil
}
