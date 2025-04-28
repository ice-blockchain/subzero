// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"fmt"
	"log"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/cenkalti/backoff/v4"
	"github.com/cockroachdb/errors"
	"google.golang.org/api/option"

	"github.com/ice-blockchain/subzero/model"
)

const (
	initialBackoffInterval = 300 * time.Millisecond
	backoffMultiplier      = 2.5
	maxBackoffInterval     = 2 * time.Second
	maxRetries             = 3
	requestDeadline        = 30 * time.Second
)

type (
	DeviceRegistrationEvent = model.Event
	DeviceID                string
	SubscriptionTopic       string
	Client                  interface {
		SendSingle(ctx context.Context, notification *Notification[*DeviceRegistrationEvent]) error
		SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error
	}
	notificationClient struct {
		client *messaging.Client
		retry  RetryConfig
	}
	Notification[TARGET SubscriptionTopic | *DeviceRegistrationEvent] struct {
		Data     map[string]interface{} `json:"data,omitempty"`
		Target   TARGET
		Title    string `json:"title,omitempty"`
		Body     string `json:"body,omitempty"`
		ImageURL string `json:"imageUrl,omitempty"`
	}
	RetryConfig struct {
		MaxRetries  int
		InitialWait time.Duration
		MaxWait     time.Duration
	}
	Option func(*options)

	options struct {
		credentialsFile string
		credentialsJSON []byte
		retryConfig     RetryConfig
	}
)

var (
	ErrInvalidDeviceToken = errors.New("device token is invalid")
	defaultRetryConfig    = RetryConfig{
		MaxRetries:  maxRetries,
		InitialWait: initialBackoffInterval,
		MaxWait:     maxBackoffInterval,
	}
)

func WithRetryConfig(config RetryConfig) Option {
	return func(o *options) {
		o.retryConfig = config
	}
}

func WithCredentialsFile(filePath string) Option {
	return func(o *options) {
		o.credentialsFile = filePath
	}
}

func WithCredentialsJSON(jsonStr string) Option {
	return func(o *options) {
		o.credentialsJSON = []byte(jsonStr)
	}
}

func IsInvalidDeviceToken(err error) bool {
	return errors.Is(err, ErrInvalidDeviceToken)
}

func New(ctx context.Context, opts ...Option) (Client, error) {
	options := &options{
		retryConfig: defaultRetryConfig,
	}

	for _, opt := range opts {
		opt(options)
	}

	var (
		app *firebase.App
		err error
	)

	if len(options.credentialsJSON) > 0 {
		app, err = firebase.NewApp(ctx, nil, option.WithCredentialsJSON(options.credentialsJSON))
	} else if options.credentialsFile != "" {
		app, err = firebase.NewApp(ctx, nil, option.WithCredentialsFile(options.credentialsFile))
	} else {
		return nil, fmt.Errorf("neither credentials file nor JSON provided")
	}

	if err != nil {
		return nil, err
	}

	fcmClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}

	s := &notificationClient{
		client: fcmClient,
		retry:  options.retryConfig,
	}

	return s, nil
}

func (s *notificationClient) sendWithRetry(ctx context.Context, message *messaging.Message) (string, error) {
	var id string
	err := retry(ctx, func() error {
		var err error
		id, err = s.client.Send(ctx, message)
		return err
	})

	if err != nil {
		if messaging.IsInvalidArgument(err) || messaging.IsUnregistered(err) || messaging.IsSenderIDMismatch(err) {
			return "", ErrInvalidDeviceToken
		}
		return "", fmt.Errorf("fcm send failed for %#v: %w", message, err)
	}

	return id, nil
}

func (s *notificationClient) SendSingle(ctx context.Context, notification *Notification[*DeviceRegistrationEvent]) error {
	token := notification.Target.GetTag("token")
	if token == nil || token.Value() == "" {
		return nil
	}
	data := make(map[string]string)
	for k, v := range notification.Data {
		if str, ok := v.(string); ok {
			data[k] = str
		} else {
			data[k] = fmt.Sprintf("%v", v)
		}
	}
	message := &messaging.Message{
		Token: token.Value(),
		Data:  data,
	}

	if notification.Title != "" || notification.Body != "" || notification.ImageURL != "" {
		message.Notification = &messaging.Notification{
			Title:    notification.Title,
			Body:     notification.Body,
			ImageURL: notification.ImageURL,
		}
	}

	_, err := s.sendWithRetry(ctx, message)
	if err != nil {
		return err
	}

	return nil
}

func (s *notificationClient) SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error {
	data := make(map[string]string)
	for k, v := range notification.Data {
		if str, ok := v.(string); ok {
			data[k] = str
		} else {
			data[k] = fmt.Sprintf("%v", v)
		}
	}

	message := &messaging.Message{
		Topic: string(notification.Target),
		Data:  data,
	}

	if notification.Title != "" || notification.Body != "" || notification.ImageURL != "" {
		message.Notification = &messaging.Notification{
			Title:    notification.Title,
			Body:     notification.Body,
			ImageURL: notification.ImageURL,
		}
	}

	_, err := s.sendWithRetry(ctx, message)

	return errors.Wrap(err, "failed to send topic notification")
}

func retry(ctx context.Context, op func() error) error {
	return backoff.RetryNotify(
		op,
		backoff.WithContext(&backoff.ExponentialBackOff{
			InitialInterval:     initialBackoffInterval,
			RandomizationFactor: 0.5,
			Multiplier:          backoffMultiplier,
			MaxInterval:         maxBackoffInterval,
			MaxElapsedTime:      requestDeadline,
			Stop:                backoff.Stop,
			Clock:               backoff.SystemClock,
		}, ctx),
		func(e error, next time.Duration) {
			log.Printf("FCM call failed. retrying in %v... Error: %v", next, e)
		})
}
