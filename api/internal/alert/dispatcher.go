package alert

import (
	"context"
	"log/slog"
	"time"
)

// Voice turns a message into spoken audio and returns a URL for it (relative to the API root).
type Voice interface {
	AudioURL(ctx context.Context, text, lang string) (string, error)
}

// Dispatcher sends an event's plan through a Notifier, one recipient at a
// time in plan order (nearest place first), and records each outcome on the
// event. With a Voice, every message also gets a spoken version first.
type Dispatcher struct {
	Notifier Notifier
	Voice    Voice // optional
	Log      *slog.Logger
	Now      func() time.Time
}

func NewDispatcher(n Notifier) *Dispatcher {
	return &Dispatcher{Notifier: n, Now: time.Now}
}

func (d *Dispatcher) Dispatch(ctx context.Context, ev *Event) {
	audio := map[string]string{} // one synthesis per distinct text, shared by everyone at that place
	for i := range ev.Recipients {
		r := &ev.Recipients[i]
		if d.Voice != nil {
			key := r.Lang + "|" + r.Message
			if _, done := audio[key]; !done {
				url, err := d.Voice.AudioURL(ctx, r.Message, r.Lang)
				if err != nil && d.Log != nil {
					d.Log.Warn("voice synthesis failed; sending text only", "event", ev.ID, "err", err)
				}
				audio[key] = url
			}
			r.AudioURL = audio[key]
		}
		if ev.DryRun {
			at := d.Now().UTC()
			r.Delivery = Delivery{Status: DeliveryDryRun, At: &at}
			continue
		}
		err := d.Notifier.Send(ctx, Message{
			EventID: ev.ID, SubscriptionID: r.SubscriptionID,
			To: r.Phone, Channel: r.Channel, Lang: r.Lang,
			Title: title(r.Lang), Text: r.Message, Short: r.Short, AudioURL: r.AudioURL,
			LakeID: ev.LakeID, PlaceOSM: r.PlaceOSM, ArrivalMinFast: r.ArrivalMinFast,
			Push: r.push,
		})
		at := d.Now().UTC()
		r.Delivery = Delivery{Status: DeliverySent, At: &at}
		if err != nil {
			r.Delivery = Delivery{Status: DeliveryFailed, Error: err.Error(), At: &at}
		}
	}
	ev.Summary = summarize(ev.Recipients)
}

func summarize(rs []Recipient) Summary {
	s := Summary{Recipients: len(rs)}
	for _, r := range rs {
		switch r.Delivery.Status {
		case DeliverySent:
			s.Sent++
		case DeliveryFailed:
			s.Failed++
		}
	}
	return s
}
