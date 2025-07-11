// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jellydator/ttlcache/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Config struct {
		PrivateKey string `yaml:"private-key" validate:"required"`
		RelayURL   string `yaml:"relay-url"   validate:"required,url"`
	}

	Option func(*dvm)

	jobItem interface {
		Process(ctx context.Context, e *model.Event) (result string, err error)
		RequiredPaymentAmount() float64
		IsBidAmountEnough(amount string) bool
	}

	jobInfo struct {
		Result chan *model.Event
		Event  *model.Event
		Cancel context.CancelFunc
	}

	dvm struct {
		WG            *sync.WaitGroup
		Jobs          *xsync.Map[string, *jobInfo]
		ResponseCache *ttlcache.Cache[string, *xsync.Map[string, *model.Event]]
		Config        *Config
		PublicKey     string
	}
)

const (
	jobTimeoutDeadline = 1 * time.Minute
	logThreshold       = 100 * time.Millisecond
)

func mustNewDVM(ctx context.Context, opts ...Option) *dvm {
	var err error

	server := &dvm{
		WG:            new(sync.WaitGroup),
		Config:        cfg.MustGet[Config](),
		Jobs:          xsync.NewMap[string, *jobInfo](),
		ResponseCache: ttlcache.New(ttlcache.WithTTL[string, *xsync.Map[string, *model.Event]](model.DVMJobResultExpiration)),
	}

	for i := range opts {
		opts[i](server)
	}

	server.PublicKey, err = model.GetPublicKey(server.Config.PrivateKey)
	if err != nil {
		log.Panicf("DVM: can't get public key from private key: %v", err)
	}

	go server.ResponseCache.Start()
	go func() {
		<-ctx.Done()
		log.Printf("DVM: shutting down")
		server.WG.Wait()
		server.ResponseCache.Stop()
	}()

	return server
}

func (d *dvm) SubmitResult(ctx context.Context, task *jobInfo, result *model.Event) {
	d.acceptDVMResponseEvent(result)
	select {
	case <-time.After(time.Minute):
		log.Printf("DVM: job %v: result submission timeout", task.Event.ID)
	case <-ctx.Done():
	case task.Result <- result:
	}
}

func (d *dvm) AcceptJob(ctx context.Context, event *model.Event) (<-chan *model.Event, error) {
	if (event.Kind < 5000 && event.Kind != nostr.KindDeletion) || event.Kind > nostr.KindJobFeedback {
		return nil, nil
	}

	if event.Kind == nostr.KindDeletion {
		return nil, d.handleDeletionEvent(ctx, event)
	}

	// Disabled for now.
	if false {
		if event.GetTag("p").Value() != d.PublicKey {
			log.Printf("dvm job is not for specified for this service provider: %v", event)
			return nil, nil
		}
	}

	for _, tag := range event.Tags {
		if tag.Key() == "relays" {
			// TODO: remove later.
			log.Panicf("DVM: job %v: found %q tag", event.ID, tag.Key())
		}
	}

	ctx, cancel := context.WithTimeout(ctx, jobTimeoutDeadline)
	task := &jobInfo{
		Result: make(chan *model.Event, 1),
		Event:  event,
		Cancel: cancel,
	}
	d.Jobs.Store(event.ID, task)

	d.WG.Add(1)
	go func() {
		defer d.Jobs.Delete(event.ID)
		defer cancel()

		d.execute(ctx, task)
		d.WG.Done()
		close(task.Result)
	}()

	return task.Result, nil
}

func (d *dvm) handleDeletionEvent(ctx context.Context, event *model.Event) error {
	stopEventID := event.GetTag("e").Value()
	kTag := event.GetTag("k").Value()
	if stopEventID == "" || kTag == "" {
		return nil
	}

	log.Printf("DVM: job delete request: %v / %v", stopEventID, kTag)

	stopEventKind, err := strconv.Atoi(kTag)
	if err != nil {
		return errors.Wrapf(err, "can't parse stop event kind for id: %v", stopEventID)
	} else if stopEventKind < 5000 || stopEventKind > 5999 {
		return nil
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, jobTimeoutDeadline)
	defer reqCancel()

	return errors.Wrapf(d.stopEvent(reqCtx, event, stopEventID), "dvm job deletion kind, failed to stop event with id %q and kind %d: %v", stopEventID, stopEventKind, err)
}

func (d *dvm) execute(ctx context.Context, task *jobInfo) {
	var job jobItem

	switch task.Event.Kind {
	case model.KindJobNostrEventCount:
		job = newNostrEventCountJob(d)

	default:
		log.Printf("DVM: job %v: kind: %v: not supported", task.Event.ID, task.Event.Kind)

		return
	}

	start := time.Now()
	defer func() {
		if elapsed := time.Since(start); elapsed > logThreshold {
			log.Printf("DVM: job %v: processed in %v", task.Event.ID, elapsed)
		}
	}()

	bidTag := task.Event.GetTag("bid")
	if bidTag != nil && !job.IsBidAmountEnough(bidTag.Value()) {
		if err := d.publishJobFeedback(ctx, task, model.JobFeedbackStatusPaymentRequired, "Bid amount is not enough", job.RequiredPaymentAmount()); err != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, err)
		}
		return
	}

	jobResult, err := job.Process(ctx, task.Event)
	if errors.Is(ctx.Err(), context.Canceled) {
		log.Printf("DVM: job %v: canceled", task.Event.ID)

		return
	}

	if err != nil {
		if fErr := d.publishJobFeedback(ctx, task, model.JobFeedbackStatusError, "error: "+err.Error(), job.RequiredPaymentAmount()); fErr != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, fErr)
		}
		return
	}

	result, err := d.finalizeJob(task.Event, jobResult, job.RequiredPaymentAmount())
	if err != nil {
		if fErr := d.publishJobFeedback(ctx, task, model.JobFeedbackStatusError, "error: "+err.Error(), job.RequiredPaymentAmount()); fErr != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, fErr)
		}
		return
	}

	if err := d.publishJobResult(ctx, task, result); err != nil {
		if fErr := d.publishJobFeedback(ctx, task, model.JobFeedbackStatusError, "error: "+err.Error(), job.RequiredPaymentAmount()); fErr != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, fErr)
		}
	}
}

func (d *dvm) finalizeJob(incomingEvent *model.Event, payload string, reqiredPaymentAmount float64) (*model.Event, error) {
	var result model.Event

	result.CreatedAt = nostr.Now()
	result.Content = payload
	result.Kind = incomingEvent.Kind + 1000
	result.Tags = model.Tags{
		{"request", incomingEvent.String()},
		{"e", incomingEvent.ID, d.Config.RelayURL},
		{"expiration", result.CreatedAt.Add(model.DVMJobResultExpiration).String()},
		{"p", incomingEvent.GetMasterPublicKey()},
		{model.CustomIONTagOnBehalfOf, d.PublicKey},
	}

	if reqiredPaymentAmount > 0 {
		result.Tags = append(result.Tags, model.Tag{"amount", strconv.FormatFloat(reqiredPaymentAmount, 'f', -1, 64)})
	}

	if err := result.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return nil, errors.Wrapf(err, "failed to sign event: %v", result)
	}

	return &result, nil
}

func (d *dvm) publishJobResult(ctx context.Context, task *jobInfo, result *model.Event) error {
	var wg sync.WaitGroup

	d.SubmitResult(ctx, task, result)

	relays := connectToRelays(ctx, task.Event.ID, collectTargetRelayURLsFromEvent(task.Event))
	if len(relays) == 0 {
		return nil
	}
	defer closeRelays(relays)

	wg.Add(len(relays))
	successfull := atomic.Int32{}
	for _, relay := range relays {
		go func() {
			defer wg.Done()

			err := relay.Publish(ctx, result.Event)
			if err != nil && strings.Contains(err.Error(), "auth-required:") {
				err = errors.Wrap(relay.Auth(ctx, func(event *nostr.Event) error {
					subZeroEvent := model.Event{Event: *event}
					if err := subZeroEvent.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
						return err
					}
					*event = subZeroEvent.Event

					return nil
				}), "failed to authenticate to relay")
				if err == nil {
					err = relay.Publish(ctx, result.Event)
				}
			}
			if err != nil {
				log.Printf("DVM: job %v: failed to publish job result to relay: %v, err: %v", task.Event.ID, relay, err)
				return
			}
			successfull.Add(1)
		}()
	}

	wg.Wait()

	if len(relays) > 0 && successfull.Load() == 0 && ctx.Err() == nil {
		return errors.Errorf("failed to publish job result to %d relay(s)", len(relays))
	}
	return nil
}

func (d *dvm) stopEvent(ctx context.Context, event *model.Event, stopJobID string) error {
	jobInfo, ok := d.Jobs.LoadAndDelete(stopJobID)
	if !ok {
		log.Printf("DVM: job stop: job %s not found", stopJobID)

		return nil
	}

	jobInfo.Cancel()

	return errors.Wrapf(d.publishJobFeedback(
		ctx,
		jobInfo,
		model.JobFeedbackStatusError,
		fmt.Sprintf("Job %s has been stopped", stopJobID),
		0,
	), "can't publish error feedback: %v", event)
}

func (d *dvm) publishJobFeedback(ctx context.Context, task *jobInfo, status model.JobFeedbackStatus, payload string, reqiredPaymentAmount float64) error {
	var event model.Event

	event.Kind = nostr.KindJobFeedback
	event.CreatedAt = nostr.Now()
	event.Content = payload
	event.Tags = model.Tags{
		{"status", string(status)},
		{"expiration", event.CreatedAt.Add(model.DVMJobResultExpiration).String()},
		{"e", task.Event.ID},
		{"p", task.Event.GetMasterPublicKey()},
		{model.CustomIONTagOnBehalfOf, d.PublicKey},
	}

	if reqiredPaymentAmount > 0 {
		event.Tags = append(event.Tags, nostr.Tag{"amount", strconv.FormatFloat(reqiredPaymentAmount, 'f', -1, 64)})
	}

	if err := event.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return errors.Wrapf(err, "failed to sign event: %v", event)
	}

	return d.publishJobResult(ctx, task, &event)
}

func connectToRelays(ctx context.Context, jobID string, relayList []string) (resultRelays []*nostr.Relay) {
	if len(relayList) == 0 {
		return nil
	}

	for _, relayUrl := range relayList {
		relay, err := connectToRelay(ctx, relayUrl)
		if err != nil {
			log.Printf("DVM: job %v: error: failed to connect to relay: %v, err: %v", jobID, relayUrl, err)
		} else {
			resultRelays = append(resultRelays, relay)
		}
	}
	return resultRelays
}

func connectToRelay(ctx context.Context, url string) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url, nostr.WithSignatureChecker(func(e *nostr.Event) bool {
		subzeroEvent := model.Event{Event: *e}
		ok, _ := subzeroEvent.CheckSignature()

		return ok
	}))

	err := relay.Connect(ctx)
	if err != nil {
		return nil, err
	}

	return relay, nil
}

func closeRelays(relayList []*nostr.Relay) {
	for _, r := range relayList {
		if err := r.Close(); err != nil {
			log.Printf("[DVM]: can't close relay:%v, err:%v", r.URL, err)
			continue
		}
	}
}

func collectSourceRelayURLsFromEvent(e *model.Event, selfURL string) (relayList []string) {
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "relay" {
			for _, relayURL := range tag[2:] {
				if selfURL != "" && relayURL == selfURL {
					continue
				}
				relayList = append(relayList, relayURL)
			}
		}
	}
	return relayList
}

func collectTargetRelayURLsFromEvent(e *model.Event) (relayList []string) {
	for _, tag := range e.Tags {
		if tag.Key() == "relays" && len(tag) > 1 {
			for ix := 1; ix < len(tag); ix++ {
				relayList = append(relayList, tag[ix])
			}
		}
	}
	return relayList
}
