package alert

import (
	"context"
	"time"
)

// Dispatcher sends an event's plan through a Notifier, one recipient at a
// time in plan order (nearest place first), and records each outcome on the
// event.
type Dispatcher struct {
	Notifier Notifier
	Now      func() time.Time
}

func NewDispatcher(n Notifier) *Dispatcher {
	return &Dispatcher{Notifier: n, Now: time.Now}
}

func (d *Dispatcher) Dispatch(ctx context.Context, ev *Event) {
	for i := range ev.Recipients {
		r := &ev.Recipients[i]
		err := d.Notifier.Send(ctx, Message{
			EventID: ev.ID, SubscriptionID: r.SubscriptionID,
			To: r.Phone, Channel: r.Channel, Lang: r.Lang, Text: r.Message,
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
