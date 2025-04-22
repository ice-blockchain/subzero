// SPDX-License-Identifier: ice License 1.0


package internal

import (
	"context"
	"errors"
	"fmt"
	"log"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"

	"github.com/ice-blockchain/subzero/model"
)

type (
	DeviceID          string
	SubscriptionTopic string
	Client            interface {
		SendSingle(ctx context.Context, notification *Notification[*model.Event]) error
		SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error
	}
	Service struct {
		client *messaging.Client
		dryRun bool
	}
	Notification[TARGET SubscriptionTopic | *model.Event] struct {
		Data     map[string]interface{} `json:"data,omitempty"`
		Target   TARGET
		Title    string `json:"title,omitempty"`
		Body     string `json:"body,omitempty"`
		ImageURL string `json:"imageUrl,omitempty"`
	}
	ServiceOption func(*Service)
)

var (
	ErrInvalidDeviceToken = errors.New("device token is invalid")
)

func WithDryRun(dryRun bool) ServiceOption {
	return func(s *Service) {
		s.dryRun = dryRun
	}
}

func IsInvalidDeviceToken(err error) bool {
	return errors.Is(err, ErrInvalidDeviceToken)
}

func New(ctx context.Context, credentialsFile string, opts ...ServiceOption) (Client, error) {
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}
	fcmClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}
	s := &Service{
		client: fcmClient,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Service) SendSingle(ctx context.Context, notification *Notification[*model.Event]) error {
	data := make(map[string]string)
	for k, v := range notification.Data {
		if str, ok := v.(string); ok {
			data[k] = str
		} else {
			data[k] = fmt.Sprintf("%v", v)
		}
	}
	token := notification.Target.GetTag("token")
	if token == nil || token.Value() == "" {
		return nil
	}

	message := &messaging.Message{
		Token: token.Value(),
		Notification: &messaging.Notification{
			Title:    notification.Title,
			Body:     notification.Body,
			ImageURL: notification.ImageURL,
		},
		Data: data,
	}
	if s.dryRun {
		_, err := s.client.SendDryRun(ctx, message)

		return handleError(err, message)
	}
	_, err := s.client.Send(ctx, message)

	return handleError(err, message)
}

func (s *Service) SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error {
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
		Notification: &messaging.Notification{
			Title:    notification.Title,
			Body:     notification.Body,
			ImageURL: notification.ImageURL,
		},
		Data: data,
	}
	if s.dryRun {
		_, err := s.client.SendDryRun(ctx, message)

		return handleError(err, message)
	}
	_, err := s.client.Send(ctx, message)

	return handleError(err, message)
}

func handleError(err error, message *messaging.Message) error {
	if err != nil {
		if messaging.IsInvalidArgument(err) || messaging.IsUnregistered(err) || messaging.IsSenderIDMismatch(err) {
			return ErrInvalidDeviceToken
		} else {
			rErr := fmt.Errorf("fcm send failed for %#v, %v", message, err)
			log.Print(rErr)

			return rErr
		}
	}

	return nil
}
