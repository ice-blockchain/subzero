// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/cenkalti/backoff/v4"
	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/rs/zerolog/log"
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
		client     *messaging.Client
		privateKey string
		retry      RetryConfig
	}
	Notification[TARGET SubscriptionTopic | *DeviceRegistrationEvent] struct {
		Data     map[string]interface{} `json:"data,omitempty"`
		Target   TARGET
		Title    string `json:"title,omitempty"`
		Body     string `json:"body,omitempty"`
		ImageURL string `json:"imageUrl,omitempty"`
		Kind     int    `json:"kind,omitempty"`
	}
	RetryConfig struct {
		MaxRetries  int
		InitialWait time.Duration
		MaxWait     time.Duration
	}
	Option func(*options)

	options struct {
		credentialsFile string
		privateKey      string
		credentialsJSON []byte
		retryConfig     RetryConfig
	}
)

var (
	ErrInvalidDeviceToken = errors.New("device token is invalid")
	ErrMessageTooLarge    = errors.New("message is too large")
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

func WithPrivateKey(privateKey string) Option {
	return func(o *options) {
		o.privateKey = privateKey
	}
}

func IsInvalidDeviceToken(err error) bool {
	return errors.Is(err, ErrInvalidDeviceToken)
}

func IsMessageTooLarge(err error) bool {
	return errors.Is(err, ErrMessageTooLarge)
}

func isUnregisteredByContent(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	unregisteredPatterns := []string{
		"requested entity was not found",
		"registration token is not a valid fcm registration token",
		"unregistered",
	}
	for _, pattern := range unregisteredPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
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
		return nil, errors.Wrap(err, "failed to create firebase app")
	}

	fcmClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create fcm client")
	}
	x25519PrivateKey, err := nip44.ConvertEd25519PrivateKeyToX25519(options.privateKey)
	if err != nil {
		panic("failed to convert private key to x25519: " + err.Error())
	}

	s := &notificationClient{
		client:     fcmClient,
		retry:      options.retryConfig,
		privateKey: x25519PrivateKey,
	}

	return s, nil
}

func (s *notificationClient) sendWithRetry(ctx context.Context, message *messaging.Message, kind int) (string, error) {
	var id string
	err := retry(ctx, func() error {
		var err error
		id, err = s.client.Send(ctx, message)
		if err != nil {
			if strings.Contains(err.Error(), "message is too big") {
				return &backoff.PermanentError{Err: ErrMessageTooLarge}
			}
			if messaging.IsInvalidArgument(err) || messaging.IsUnregistered(err) || messaging.IsSenderIDMismatch(err) {
				return &backoff.PermanentError{Err: ErrInvalidDeviceToken}
			}
			if isUnregisteredByContent(err) {
				return &backoff.PermanentError{Err: ErrInvalidDeviceToken}
			}
			// TODO: specify the exact error string.
			if strings.Contains(strings.ToLower(err.Error()), "400 bad request") {
				return &backoff.PermanentError{Err: ErrInvalidDeviceToken}
			}
		}

		return err
	})

	if err != nil {
		if IsMessageTooLarge(err) {
			return "", errors.Wrapf(err, "message is too large, kind: %d, size: %d bytes", kind, calculateMessageSize(message))
		}
		if IsInvalidDeviceToken(err) {
			return "", ErrInvalidDeviceToken
		}
		return "", fmt.Errorf("fcm send failed for %#v: %w", message, err)
	}

	return id, nil
}

func (s *notificationClient) createSingleMessage(notification *Notification[*DeviceRegistrationEvent]) (*messaging.Message, error) {
	tokenTag := notification.Target.GetTag("token")
	if tokenTag == nil || tokenTag.Value() == "" {
		return nil, nil
	}
	decryptedToken, err := DecryptToken(notification.Target, s.privateKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decrypt token for device registration event: %s", notification.Target.ID)
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
		Token: decryptedToken,
		Data:  data,
	}

	if notification.Title != "" || notification.Body != "" || notification.ImageURL != "" {
		message.Notification = &messaging.Notification{
			Title:    notification.Title,
			Body:     notification.Body,
			ImageURL: notification.ImageURL,
		}
	}

	return message, nil
}

func (s *notificationClient) SendSingle(ctx context.Context, notification *Notification[*DeviceRegistrationEvent]) error {
	message, err := s.createSingleMessage(notification)
	if err != nil {
		return err
	}
	if message == nil {
		return nil
	}
	_, err = s.sendWithRetry(ctx, message, notification.Kind)
	if err != nil {
		return err
	}

	return nil
}

func (s *notificationClient) createTopicMessage(notification *Notification[SubscriptionTopic]) *messaging.Message {
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

	return message
}

func (s *notificationClient) SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error {
	message := s.createTopicMessage(notification)
	_, err := s.sendWithRetry(ctx, message, notification.Kind)

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
			log.Error().Err(e).Dur("retry_delay", next).Msg("FCM call failed. retrying")
		})
}

func DecryptToken(ev *model.Event, privateKey string) (string, error) {
	token := ev.GetTag("token")
	if token == nil {
		return "", nil
	}
	pubkeyX25519, err := nip44.ConvertEd25519PublicKeyToX25519(ev.PubKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert pubkey to x25519")
	}
	conversationKey, err := nip44.GenerateConversationKeyX25519(privateKey, pubkeyX25519)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate conversation key")
	}
	decryptedToken, err := nip44.DecryptX25519(token.Value(), conversationKey)
	if err != nil {
		return "", errors.Wrapf(err, "failed to decrypt token for event: %s", ev.ID)
	}

	return decryptedToken, nil
}

func calculateMessageSize(message *messaging.Message) int {
	data, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal message")

		return 0
	}

	return len(data)
}
