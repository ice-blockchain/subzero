// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
	"golang.org/x/sync/errgroup"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

type (
	JobFeedbackStatus = string
	JobItem           interface {
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
		dvmProcessCancel *xsync.MapOf[string, *dvmProcessCancelInfo]
		privateKey       string
		relayConnectTLS  *tls.Config
	}
	config struct {
		PrivateKey string `yaml:"private-key"`
		TLSCert    string `yaml:"tls-cert"`
		TLSKey     string `yaml:"tls-key"`
	}
)

var (
	jobTimeoutDeadline = 1 * time.Minute
	globalDVM          *dvm
	globalConfig       *config
)

func MustInit() {
	globalConfig = cfg.MustGet[config]()
	globalDVM = &dvm{
		dvmProcessCancel: xsync.NewMapOf[string, *dvmProcessCancelInfo](),
		privateKey:       globalConfig.PrivateKey,
	}
	if globalConfig.TLSKey != "-" && globalConfig.TLSCert != "-" {
		globalDVM.relayConnectTLS = buildTLS()
	}
}

func buildTLS() *tls.Config {
	cert, err := tls.X509KeyPair([]byte(globalConfig.TLSCert), []byte(globalConfig.TLSKey))
	if err != nil {
		log.Panic(err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM([]byte(globalConfig.TLSCert)); !ok {
		log.Panic(errors.New("failed to append tls to cert pool"))
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}
}

func AcceptJob(ctx context.Context, event *model.Event) error {
	return globalDVM.AcceptJob(ctx, event)
}

func (d *dvm) AcceptJob(ctx context.Context, event *model.Event) error {
	if (event.Kind < 5000 && event.Kind != nostr.KindDeletion) || event.Kind > 5999 {
		return nil
	}
	if err := event.Validate(); err != nil {
		return errors.Wrapf(err, "wrong dvm job: %v", event)
	}
	if event.Kind == nostr.KindDeletion {
		if err := d.handleDeletionEvent(ctx, event); err != nil {
			return errors.Wrapf(err, "can't handle deletion event: %v", err)
		}

		return nil
	}
	if false {
		res, err := d.isServiceProviderCustomerInterestedIn(event)
		if err != nil {
			log.Print("can't check if service provider is interested: ", err)

			return nil
		}
		if !res {
			log.Printf("dvm job is not for specified for this service provider: %v", event)

			return nil
		}
	}
	go d.process(ctx, event)

	return nil
}

func (d *dvm) isServiceProviderCustomerInterestedIn(event *model.Event) (res bool, err error) {
	pubKey, err := nostr.GetPublicKey(d.privateKey)
	if err != nil {
		return false, errors.Wrap(err, "can't get public key")
	}
	tag := event.GetTag("p")

	return tag != nil && tag.Value() == pubKey, nil
}

func (d *dvm) handleDeletionEvent(ctx context.Context, event *model.Event) error {
	if event.Kind != nostr.KindDeletion {
		return nil
	}
	stopEventID := event.GetTag("e")
	kTag := event.GetTag("k")
	if stopEventID == nil || kTag == nil {
		return nil
	}
	stopEventKind, err := strconv.Atoi(kTag.Value())
	if err != nil {
		return errors.Wrapf(err, "can't parse stop event kind for id: %v", stopEventID)
	}
	if stopEventKind < 5000 || stopEventKind > 5999 {
		return nil
	}
	reqCtx, reqCancel := context.WithTimeout(ctx, jobTimeoutDeadline)
	defer reqCancel()

	return errors.Wrapf(d.stopEvent(reqCtx, event, stopEventID.Value()), "dvm job deletion kind, failed to stop event with id %q and kind %d: %v", stopEventID, stopEventKind, err)
}

func (d *dvm) process(ctx context.Context, event *model.Event) {
	reqCtx, reqCancel := context.WithTimeout(ctx, jobTimeoutDeadline)
	defer func() {
		reqCancel()
		d.dvmProcessCancel.Delete(event.GetID())
	}()
	var relayList []string
	for _, tag := range event.Tags {
		if tag.Key() == "relays" && len(tag) > 1 {
			for ix := 1; ix < len(tag); ix++ {
				relayList = append(relayList, tag[ix])
			}
		}
	}
	outputRelays := connectToRelays(reqCtx, relayList, d.relayConnectTLS)
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
		job = newNostrEventCountJob(outputRelays, d.privateKey, d.relayConnectTLS)
	default:
		log.Printf("dvm kind:%v not supported", event.Kind)

		return
	}
	bidTag := event.GetTag("bid")
	if bidTag != nil && !job.IsBidAmountEnough(bidTag.Value()) {
		if err := job.OnPaymentRequiredFeedback(reqCtx, event); err != nil {
			log.Printf("failed to publish job error feedback: %v", event)
		}

		return
	}
	payload, err := job.Process(reqCtx, event)
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
	encodedIncomingEvent, err := json.Marshal(incomingEvent)
	if err != nil {
		return errors.Wrapf(err, "failed to json encode incoming event: %v", incomingEvent)
	}
	result := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   payload,
			Kind:      incomingEvent.Kind + 1000,
			Tags: nostr.Tags{
				[]string{"request", string(encodedIncomingEvent)},
				[]string{"e", incomingEvent.GetID()},
				[]string{"i", incomingEvent.Content},
				[]string{"p", incomingEvent.PubKey},
			},
		},
	}
	if reqiredPaymentAmount > 0 {
		result.Tags = append(result.Tags, nostr.Tag{"amount", strconv.FormatFloat(reqiredPaymentAmount, 'f', -1, 64)})
	}
	if err := result.Sign(d.privateKey); err != nil {
		return errors.Wrapf(err, "failed to sign event: %v", result)
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
		cancelInfo.RelayList,
		d.privateKey,
		fmt.Sprintf("Job %s has been stopped", stopJobID),
		0,
	), "can't publish error feedback: %v", event)
}

func publishJobFeedback(ctx context.Context, incomingEvent *model.Event, status JobFeedbackStatus, relays []*nostr.Relay, serviceProviderPrivateKey, payload string, reqiredPaymentAmount float64) error {
	result := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
		result.Tags = append(result.Tags, nostr.Tag{"amount", strconv.FormatFloat(reqiredPaymentAmount, 'f', -1, 64)})
	}
	if err := result.Sign(serviceProviderPrivateKey); err != nil {
		return errors.Wrapf(err, "failed to sign event: %v", result)
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

func connectToRelays(ctx context.Context, relayList []string, conf *tls.Config) (resultRelays []*nostr.Relay) {
	for _, relayUrl := range relayList {
		relay, err := connectToRelay(ctx, relayUrl, conf)
		if err != nil {
			log.Printf("ERROR: failed to connect to relay: %v, err: %v", relayUrl, err)
		} else {
			resultRelays = append(resultRelays, relay)
		}
	}
	if len(relayList) > 0 {
		log.Printf("Connected to %v of %v relays", len(resultRelays), len(relayList))
	}
	return resultRelays
}

func connectToRelay(ctx context.Context, url string, conf *tls.Config) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url)
	err := relay.ConnectWithTLS(ctx, conf)
	if err != nil {
		return nil, errors.Wrapf(err, "can't connect to the relays")
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
