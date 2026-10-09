// Package notify delivers alert messages: browser push, SMS through Amazon SNS, and a router that
// picks the provider per channel. Channels without a configured provider fall back to
// a log-only notifier, so a simulation never fails for lack of an SMS account.
package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/ayush00git/aahat/api/internal/alert"
)

// Router sends each message through the notifier registered for its channel.
type Router struct {
	ByChannel map[string]alert.Notifier
	Fallback  alert.Notifier
}

func (r *Router) Send(ctx context.Context, m alert.Message) error {
	if n, ok := r.ByChannel[m.Channel]; ok && n != nil {
		return n.Send(ctx, m)
	}
	return r.Fallback.Send(ctx, m)
}

// WebPush sends a notification to a browser PushSubscription (the villager web app).
type WebPush struct {
	PublicKey, PrivateKey string
	Subscriber            string // contact for the push service, e.g. "mailto:ops@example.org"
	Client                webpush.HTTPClient
}

// PushPayload is what the web app's service worker receives.
type PushPayload struct {
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	Lang           string   `json:"lang"`
	AudioURL       string   `json:"audio_url,omitempty"`
	LakeID         string   `json:"lake_id"`
	PlaceOSM       string   `json:"place_osm"`
	ArrivalMinFast *float64 `json:"arrival_min_fast,omitempty"`
	EventID        string   `json:"event_id"`
}

func (w *WebPush) Send(ctx context.Context, m alert.Message) error {
	var sub webpush.Subscription
	if err := json.Unmarshal(m.Push, &sub); err != nil || sub.Endpoint == "" {
		return fmt.Errorf("webpush: no valid push subscription")
	}
	body, err := json.Marshal(PushPayload{
		Title: m.Title, Body: m.Text, Lang: m.Lang, AudioURL: m.AudioURL,
		LakeID: m.LakeID, PlaceOSM: m.PlaceOSM, ArrivalMinFast: m.ArrivalMinFast, EventID: m.EventID,
	})
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, body, &sub, &webpush.Options{
		HTTPClient:      w.Client,
		Subscriber:      w.Subscriber,
		VAPIDPublicKey:  w.PublicKey,
		VAPIDPrivateKey: w.PrivateKey,
		TTL:             3600,
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return fmt.Errorf("webpush: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webpush: push service answered %s", resp.Status)
	}
	return nil
}

// SNSPublisher is the part of the SNS client used here (a fake in tests).
type SNSPublisher interface {
	Publish(ctx context.Context, in *sns.PublishInput, opts ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// SNSSMS sends SMS through Amazon SNS. To India without DLT registration this goes over
// international (ILDO) routes; in the SNS sandbox only verified numbers receive it.
type SNSSMS struct {
	Client SNSPublisher
}

func (s *SNSSMS) Send(ctx context.Context, m alert.Message) error {
	text := m.Text
	if m.Short != "" {
		text = m.Short // one SMS part where possible: Hindi SMS parts hold only 70 characters
	}
	_, err := s.Client.Publish(ctx, &sns.PublishInput{
		PhoneNumber: aws.String(m.To),
		Message:     aws.String(text),
		MessageAttributes: map[string]snstypes.MessageAttributeValue{
			"AWS.SNS.SMS.SMSType": {DataType: aws.String("String"), StringValue: aws.String("Transactional")},
		},
	})
	if err != nil {
		return fmt.Errorf("sns sms: %w", err)
	}
	return nil
}
