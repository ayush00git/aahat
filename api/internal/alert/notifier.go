package alert

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// Message is one warning to one phone.
type Message struct {
	EventID        string `json:"event_id"`
	SubscriptionID string `json:"subscription_id"`
	To             string `json:"to"`
	Channel        string `json:"channel"`
	Lang           string `json:"lang"`
	Text           string `json:"text"`
}

// Notifier delivers a message over SMS, voice or WhatsApp. Implementations
// for real providers live elsewhere; Send should return once the provider has
// accepted (or rejected) the message.
type Notifier interface {
	Send(ctx context.Context, m Message) error
}

// LogNotifier records messages instead of sending them. Used for local
// development, simulations and tests.
type LogNotifier struct {
	log  *slog.Logger
	mu   sync.Mutex
	sent []Message
}

// NewLogNotifier returns a LogNotifier that also logs each message to l (nil
// for silence).
func NewLogNotifier(l *slog.Logger) *LogNotifier { return &LogNotifier{log: l} }

func (n *LogNotifier) Send(_ context.Context, m Message) error {
	n.mu.Lock()
	n.sent = append(n.sent, m)
	n.mu.Unlock()
	if n.log != nil {
		n.log.Info("alert message (not sent: log notifier)",
			"event", m.EventID, "to", maskPhone(m.To), "channel", m.Channel, "lang", m.Lang, "text", m.Text)
	}
	return nil
}

// Sent returns a copy of the messages recorded so far.
func (n *LogNotifier) Sent() []Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]Message(nil), n.sent...)
}

// maskPhone keeps logs free of full phone numbers: +919812345678 -> +9198******78.
func maskPhone(p string) string {
	if len(p) < 7 {
		return "***"
	}
	return p[:5] + strings.Repeat("*", len(p)-7) + p[len(p)-2:]
}
