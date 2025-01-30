// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
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
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

type (
	JobFeedbackStatus = string
	JobItem           interface {
		Process(ctx context.Context, e *model.Event) (result string, err error)
		RequiredPaymentAmount() float64
		IsBidAmountEnough(amount string) bool
	}

	jobInfo struct {
		Event        *model.Event
		OutputRelays []string
		Cancel       context.CancelFunc
	}

	dvm struct {
		Jobs            *xsync.MapOf[string, *jobInfo]
		RelayConnectTLS *tls.Config
		PrivateKey      string
		dvmResponses    *ttlcache.Cache[string, *xsync.MapOf[string, *model.Event]]
	}
	config struct {
		PrivateKey string `yaml:"private-key"`
		TLSCert    string `yaml:"tls-cert"`
		TLSKey     string `yaml:"tls-key"`
		RelayURL   string `yaml:"relay-url" validate:"required,url"`
	}
)

var (
	jobTimeoutDeadline = 1 * time.Minute
	globalDVM          *dvm
	globalConfig       *config
)

func MustInit(ctx context.Context) {
	globalConfig = cfg.MustGet[config]()
	globalDVM = &dvm{
		Jobs:         xsync.NewMapOf[string, *jobInfo](),
		PrivateKey:   globalConfig.PrivateKey,
		dvmResponses: ttlcache.New[string, *xsync.MapOf[string, *model.Event]](ttlcache.WithTTL[string, *xsync.MapOf[string, *model.Event]](model.DVMJobResultExpiration)),
	}
	go globalDVM.dvmResponses.Start()
	if globalConfig.TLSKey != "-" && globalConfig.TLSCert != "-" {
		globalDVM.RelayConnectTLS = buildTLS()
	}
	go func() {
		<-ctx.Done()
		globalDVM.dvmResponses.Stop()
	}()
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
	if (event.Kind < 5000 && event.Kind != nostr.KindDeletion) || event.Kind > 7000 {
		return nil
	}
	if event.IsJobResponse() {
		return errors.Wrapf(d.acceptDVMResponseEvent(event), "failed to accept dvm response")
	}

	if err := validation.Validate(ctx, event); err != nil {
		return errors.Wrapf(err, "wrong dvm job: %v", event)
	}

	if event.Kind == nostr.KindDeletion {
		log.Printf("DVM: job delete request: %v", event.GetTag("e").Value())

		return d.handleDeletionEvent(ctx, event)
	}

	// Disabled for now.
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

	var relayList []string
	for _, tag := range event.Tags {
		if tag.Key() == "relays" && len(tag) > 1 {
			for ix := 1; ix < len(tag); ix++ {
				relayList = append(relayList, tag[ix])
			}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, jobTimeoutDeadline)
	task := &jobInfo{
		Event:        event,
		OutputRelays: relayList,
		Cancel:       cancel,
	}
	d.Jobs.Store(event.ID, task)

	go func() {
		defer d.Jobs.Delete(event.ID)
		defer cancel()

		d.execute(ctx, task)
	}()

	return nil
}

func (d *dvm) isServiceProviderCustomerInterestedIn(event *model.Event) (res bool, err error) {
	pubKey, err := model.GetPublicKey(d.PrivateKey)
	if err != nil {
		return false, errors.Wrap(err, "can't get public key")
	}
	tag := event.GetTag("p")

	return tag != nil && tag.Value() == pubKey, nil
}

func (d *dvm) handleDeletionEvent(ctx context.Context, event *model.Event) error {
	stopEventID := event.GetTag("e")
	kTag := event.GetTag("k")
	if stopEventID == nil || kTag == nil {
		return nil
	}

	stopEventKind, err := strconv.Atoi(kTag.Value())
	if err != nil {
		return errors.Wrapf(err, "can't parse stop event kind for id: %v", stopEventID)
	} else if stopEventKind < 5000 || stopEventKind > 5999 {
		return nil
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, jobTimeoutDeadline)
	defer reqCancel()

	return errors.Wrapf(d.stopEvent(reqCtx, event, stopEventID.Value()), "dvm job deletion kind, failed to stop event with id %q and kind %d: %v", stopEventID, stopEventKind, err)
}

func (d *dvm) execute(ctx context.Context, task *jobInfo) {
	var job JobItem

	switch task.Event.Kind {
	case model.KindJobNostrEventCount:
		job = newNostrEventCountJob(d.RelayConnectTLS)

	default:
		log.Printf("DVM: job %v: kind: %v: not supported", task.Event.ID, task.Event.Kind)

		return
	}

	bidTag := task.Event.GetTag("bid")
	if bidTag != nil && !job.IsBidAmountEnough(bidTag.Value()) {
		if err := d.publishJobFeedback(ctx, task, task.Event, model.JobFeedbackStatusPaymentRequired, "Bid amount is not enough", job.RequiredPaymentAmount()); err != nil {
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
		if fErr := d.publishJobFeedback(ctx, task, task.Event, model.JobFeedbackStatusError, "error: "+err.Error(), job.RequiredPaymentAmount()); fErr != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, fErr)
		}
		return
	}

	result, err := d.finalizeJob(task.Event, jobResult, job.RequiredPaymentAmount())
	if err != nil {
		if fErr := d.publishJobFeedback(ctx, task, task.Event, model.JobFeedbackStatusError, "error: "+err.Error(), job.RequiredPaymentAmount()); fErr != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, fErr)
		}
		return
	}

	if err := d.publishJobResult(ctx, task, result); err != nil {
		if fErr := d.publishJobFeedback(ctx, task, task.Event, model.JobFeedbackStatusError, "error: "+err.Error(), job.RequiredPaymentAmount()); fErr != nil {
			log.Printf("DVM: job %v: failed to publish job feedback: %v", task.Event.ID, fErr)
		}
	}
}

func (d *dvm) finalizeJob(incomingEvent *model.Event, payload string, reqiredPaymentAmount float64) (*model.Event, error) {
	pubKey, err := model.GetPublicKey(d.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "can't get public key")
	}
	now := time.Now()
	result := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Timestamp(now.Unix()),
			Content:   payload,
			Kind:      incomingEvent.Kind + 1000,
			Tags: model.Tags{
				{"request", incomingEvent.String()},
				{"e", incomingEvent.ID, globalConfig.RelayURL},
				{"expiration", strconv.FormatInt(now.Add(model.DVMJobResultExpiration).Unix(), 10)},
				{"p", incomingEvent.GetMasterPublicKey()},
				{model.CustomIONTagOnBehalfOf, pubKey},
			},
		},
	}

	if reqiredPaymentAmount > 0 {
		result.Tags = append(result.Tags, model.Tag{"amount", strconv.FormatFloat(reqiredPaymentAmount, 'f', -1, 64)})
	}

	if err := result.SignWithAlg(d.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return nil, errors.Wrapf(err, "failed to sign event: %v", result)
	}

	return &result, nil
}

func (d *dvm) publishJobResult(ctx context.Context, task *jobInfo, result *model.Event) error {
	var wg sync.WaitGroup

	wg.Add(len(task.OutputRelays))
	successfull := atomic.Int32{}
	for _, relay := range task.OutputRelays {
		go func() {
			defer wg.Done()

			r := nostr.NewRelay(ctx, relay)
			if err := r.ConnectWithTLS(ctx, d.RelayConnectTLS); err != nil {
				log.Printf("DVM: job %v: failed to connect to relay: %v, err: %v", task.Event.ID, relay, err)
				return
			}
			defer r.Close()

			err := r.Publish(ctx, result.Event)
			if err != nil && strings.Contains(err.Error(), "auth-required:") {
				err = errors.Wrap(r.Auth(ctx, func(event *nostr.Event) error {
					subZeroEvent := model.Event{Event: *event}
					if err := subZeroEvent.SignWithAlg(d.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
						return err
					}
					*event = subZeroEvent.Event

					return nil
				}), "failed to authenticate to relay")
				if err == nil {
					err = r.Publish(ctx, result.Event)
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

	if len(task.OutputRelays) > 0 && successfull.Load() == 0 && ctx.Err() == nil {
		return errors.Errorf("failed to publish job result to %d relay(s)", len(task.OutputRelays))
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
		event,
		model.JobFeedbackStatusError,
		fmt.Sprintf("Job %s has been stopped", stopJobID),
		0,
	), "can't publish error feedback: %v", event)
}

func (d *dvm) publishJobFeedback(ctx context.Context, task *jobInfo, incomingEvent *model.Event, status JobFeedbackStatus, payload string, reqiredPaymentAmount float64) error {
	pubKey, err := model.GetPublicKey(d.PrivateKey)
	if err != nil {
		return errors.Wrap(err, "can't get public key")
	}
	now := time.Now()
	result := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Timestamp(now.Unix()),
			Content:   payload,
			Kind:      nostr.KindJobFeedback,
			Tags: model.Tags{
				{"status", status},
				{"expiration", strconv.FormatInt(now.Add(model.DVMJobResultExpiration).Unix(), 10)},
				{"e", incomingEvent.ID},
				{"p", incomingEvent.GetMasterPublicKey()},
				{model.CustomIONTagOnBehalfOf, pubKey},
			},
		},
	}
	if reqiredPaymentAmount > 0 {
		result.Tags = append(result.Tags, nostr.Tag{"amount", strconv.FormatFloat(reqiredPaymentAmount, 'f', -1, 64)})
	}
	if err := result.SignWithAlg(d.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return errors.Wrapf(err, "failed to sign event: %v", result)
	}

	return d.publishJobResult(ctx, task, &result)
}

func connectToRelays(ctx context.Context, jobID string, relayList []string, conf *tls.Config) (resultRelays []*nostr.Relay) {
	for _, relayUrl := range relayList {
		if globalConfig != nil && globalConfig.RelayURL == relayUrl {
			// Skip connecting to self.
			continue
		}
		relay, err := connectToRelay(ctx, relayUrl, conf)
		if err != nil {
			log.Printf("DVM: job %v: error: failed to connect to relay: %v, err: %v", jobID, relayUrl, err)
		} else {
			resultRelays = append(resultRelays, relay)
		}
	}
	return resultRelays
}

func connectToRelay(ctx context.Context, url string, conf *tls.Config) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url, nostr.WithSignatureChecker(func(e *nostr.Event) bool {
		subzeroEvent := model.Event{Event: *e}
		ok, _ := subzeroEvent.CheckSignature()

		return ok
	}))
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
