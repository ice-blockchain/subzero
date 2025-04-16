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
)

type (
	DeviceID          string
	SubscriptionTopic string
	DeviceToken       struct {
		Token    string
		DeviceID DeviceID
	}
	DeviceTokens []DeviceToken
	Client       interface {
		SendSingle(ctx context.Context, notification *Notification[DeviceToken]) error
		SendBatch(ctx context.Context, notifications []*Notification[DeviceToken]) error
		SendMulticast(ctx context.Context, notification *Notification[DeviceTokens]) error
		SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error
	}
	Service struct {
		client *messaging.Client
		dryRun bool
	}
	Notification[TARGET SubscriptionTopic | DeviceToken | DeviceTokens] struct {
		Data     map[string]interface{} `json:"data,omitempty"`
		Target   TARGET
		Title    string `json:"title,omitempty"`
		Body     string `json:"body,omitempty"`
		ImageURL string `json:"imageUrl,omitempty"`
	}
	MulticastError struct {
		Err           error
		InvalidTokens map[DeviceToken]error
	}
)

var (
	ErrInvalidDeviceToken = errors.New("device token is invalid")
)

func IsInvalidDeviceToken(err error) bool {
	return errors.Is(err, ErrInvalidDeviceToken)
}

func New(ctx context.Context, credentialsFile string, dryRun bool) (Client, error) {
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
		dryRun: dryRun,
	}

	return s, nil
}

func (s *Service) SendSingle(ctx context.Context, notification *Notification[DeviceToken]) error {
	data := make(map[string]string)
	for k, v := range notification.Data {
		if str, ok := v.(string); ok {
			data[k] = str
		} else {
			data[k] = fmt.Sprintf("%v", v)
		}
	}

	message := &messaging.Message{
		Token: notification.Target.Token,
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

func (s *Service) SendBatch(ctx context.Context, notifications []*Notification[DeviceToken]) error {
	const maxBatchMessages = 500
	var allErrors []error

	for i := 0; i < len(notifications); i += maxBatchMessages {
		end := i + maxBatchMessages
		if end > len(notifications) {
			end = len(notifications)
		}
		batch := notifications[i:end]

		messages := make([]*messaging.Message, 0, len(batch))
		for ix := range batch {
			data := make(map[string]string)
			for k, v := range batch[ix].Data {
				if str, ok := v.(string); ok {
					data[k] = str
				} else {
					data[k] = fmt.Sprintf("%v", v)
				}
			}

			messages = append(messages, &messaging.Message{
				Token: batch[ix].Target.Token,
				Notification: &messaging.Notification{
					Title:    batch[ix].Title,
					Body:     batch[ix].Body,
					ImageURL: batch[ix].ImageURL,
				},
				Data: data,
			})
		}

		var (
			resp *messaging.BatchResponse
			err  error
		)
		if s.dryRun {
			resp, err = s.client.SendEachDryRun(ctx, messages)
		} else {
			resp, err = s.client.SendEach(ctx, messages)
		}
		if err != nil {
			return fmt.Errorf("can't send batch of messages: %v", err)
		}
		for _, response := range resp.Responses {
			if !response.Success {
				allErrors = append(allErrors, fmt.Errorf("notification failure for %v: %v", response.MessageID, response.Error))
			}
		}
	}
	if len(allErrors) > 0 {
		return fmt.Errorf("can't send %d from %d notifications: %v", len(allErrors), len(notifications), allErrors)
	}

	return nil
}

func (s *Service) SendMulticast(ctx context.Context, notification *Notification[DeviceTokens]) error {
	const maxTokensPerBatch = 500
	var allErrors []error
	invalidTokens := make(map[DeviceToken]error)
	tokens := notification.Target
	for i := 0; i < len(tokens); i += maxTokensPerBatch {
		end := i + maxTokensPerBatch
		if end > len(tokens) {
			end = len(tokens)
		}
		batchTokens := make([]string, 0, end-i)
		tokenToDeviceMap := make(map[string]DeviceToken, end-i)
		for _, token := range tokens[i:end] {
			batchTokens = append(batchTokens, token.Token)
			tokenToDeviceMap[token.Token] = token
		}

		data := make(map[string]string)
		for k, v := range notification.Data {
			if str, ok := v.(string); ok {
				data[k] = str
			} else {
				data[k] = fmt.Sprintf("%v", v)
			}
		}

		message := messaging.MulticastMessage{
			Tokens: batchTokens,
			Notification: &messaging.Notification{
				Title:    notification.Title,
				Body:     notification.Body,
				ImageURL: notification.ImageURL,
			},
			Data: data,
		}

		var (
			resp *messaging.BatchResponse
			err  error
		)
		if s.dryRun {
			resp, err = s.client.SendEachForMulticastDryRun(ctx, &message)
		} else {
			resp, err = s.client.SendEachForMulticast(ctx, &message)
		}
		if err != nil {
			return &MulticastError{
				Err:           fmt.Errorf("can't send multicast messages: %v", err),
				InvalidTokens: invalidTokens,
			}
		}
		for idx, response := range resp.Responses {
			if !response.Success {
				error := fmt.Errorf("notification failure for %v: %v", response.MessageID, response.Error)
				allErrors = append(allErrors, error)

				if idx < len(batchTokens) &&
					(messaging.IsInvalidArgument(response.Error) ||
						messaging.IsUnregistered(response.Error) ||
						messaging.IsSenderIDMismatch(response.Error)) {
					tokenStr := batchTokens[idx]
					deviceToken, exists := tokenToDeviceMap[tokenStr]
					if exists {
						invalidTokens[deviceToken] = ErrInvalidDeviceToken
					}
				}
			}
		}
	}
	if len(allErrors) > 0 {
		return &MulticastError{
			Err:           fmt.Errorf("can't send %d from %d notifications: %v", len(allErrors), len(notification.Target), allErrors),
			InvalidTokens: invalidTokens,
		}
	}

	return nil
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

		return err
	}
	_, err := s.client.Send(ctx, message)

	return err
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

func (e *MulticastError) Error() string {
	return e.Err.Error()
}

func (e *MulticastError) GetInvalidTokens() map[DeviceToken]error {
	return e.InvalidTokens
}
