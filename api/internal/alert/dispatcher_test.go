package alert

import (
	"context"
	"errors"
	"testing"
	"time"
)

// failingNotifier records calls and fails for one phone.
type failingNotifier struct {
	calls  []string
	failTo string
}

func (n *failingNotifier) Send(_ context.Context, m Message) error {
	n.calls = append(n.calls, m.To)
	if m.To == n.failTo {
		return errors.New("provider rejected number")
	}
	return nil
}

func TestDispatcherOrderAndDelivery(t *testing.T) {
	n := &failingNotifier{failTo: "+910000000002"}
	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	d := &Dispatcher{Notifier: n, Now: func() time.Time { return at }}
	ev := &Event{ID: "evt_1", Recipients: []Recipient{
		{SubscriptionID: "a", Phone: "+910000000001", Message: "first"},
		{SubscriptionID: "b", Phone: "+910000000002", Message: "second"},
		{SubscriptionID: "c", Phone: "+910000000003", Message: "third"},
	}}

	d.Dispatch(context.Background(), ev)

	want := []string{"+910000000001", "+910000000002", "+910000000003"}
	if len(n.calls) != 3 || n.calls[0] != want[0] || n.calls[1] != want[1] || n.calls[2] != want[2] {
		t.Fatalf("calls = %v, want %v (plan order)", n.calls, want)
	}
	wantStatus := []string{DeliverySent, DeliveryFailed, DeliverySent}
	for i, r := range ev.Recipients {
		if r.Delivery.Status != wantStatus[i] || r.Delivery.At == nil || !r.Delivery.At.Equal(at) {
			t.Errorf("recipient %d delivery = %+v", i, r.Delivery)
		}
	}
	if ev.Recipients[1].Delivery.Error != "provider rejected number" {
		t.Errorf("error not recorded: %+v", ev.Recipients[1].Delivery)
	}
	if ev.Summary != (Summary{Recipients: 3, Sent: 2, Failed: 1}) {
		t.Errorf("summary = %+v", ev.Summary)
	}
}

func TestLogNotifierRecords(t *testing.T) {
	n := NewLogNotifier(nil)
	d := NewDispatcher(n)
	ev := &Event{ID: "evt_2", Recipients: []Recipient{{SubscriptionID: "a", Phone: "+910000000001", Lang: "hi", Channel: "sms", Message: "x"}}}
	d.Dispatch(context.Background(), ev)
	sent := n.Sent()
	if len(sent) != 1 || sent[0] != (Message{EventID: "evt_2", SubscriptionID: "a", To: "+910000000001", Channel: "sms", Lang: "hi", Text: "x"}) {
		t.Errorf("sent = %+v", sent)
	}
}

func TestHindiTemplate(t *testing.T) {
	v := 60.8
	got, err := renderMessage(LangHindi, messageData{LakeName: lakeLabel(LangHindi, "लम डल"), PlaceName: "Kalsuin", ArrivalMinFast: formatMinutes(&v)})
	if err != nil {
		t.Fatal(err)
	}
	want := "चेतावनी: लम डल झील से बाढ़ का खतरा। आपके गाँव Kalsuin तक पानी लगभग 60.8 मिनट में पहुँच सकता है। तुरंत ऊँचे स्थान पर जाएँ।"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestLakeLabel(t *testing.T) {
	for _, tc := range []struct{ lang, name, want string }{
		{LangHindi, "लम डल", "लम डल झील"},
		{LangHindi, "घेपन घाट झील", "घेपन घाट झील"},
		{LangEnglish, "Lam Dal", "Lam Dal lake"},
		{LangEnglish, "Dal Lake", "Dal Lake"},
	} {
		if got := lakeLabel(tc.lang, tc.name); got != tc.want {
			t.Errorf("lakeLabel(%q, %q) = %q", tc.lang, tc.name, got)
		}
	}
}
