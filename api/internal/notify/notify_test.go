package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/ayush00git/aahat/api/internal/alert"
)

type recorder struct{ got []alert.Message }

func (r *recorder) Send(_ context.Context, m alert.Message) error {
	r.got = append(r.got, m)
	return nil
}

func TestRouterPicksNotifierByChannelAndFallsBack(t *testing.T) {
	push, fallback := &recorder{}, &recorder{}
	r := &Router{ByChannel: map[string]alert.Notifier{alert.ChannelWebPush: push}, Fallback: fallback}
	_ = r.Send(context.Background(), alert.Message{Channel: alert.ChannelWebPush})
	_ = r.Send(context.Background(), alert.Message{Channel: alert.ChannelVoice})
	if len(push.got) != 1 || len(fallback.got) != 1 {
		t.Fatalf("push=%d fallback=%d", len(push.got), len(fallback.got))
	}
}

type fakeSNS struct{ in *sns.PublishInput }

func (f *fakeSNS) Publish(_ context.Context, in *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.in = in
	return &sns.PublishOutput{}, nil
}

func TestSNSSendsTheShortTextAsTransactional(t *testing.T) {
	f := &fakeSNS{}
	err := (&SNSSMS{Client: f}).Send(context.Background(), alert.Message{To: "+919800000001", Text: "long", Short: "short"})
	if err != nil {
		t.Fatal(err)
	}
	if *f.in.Message != "short" || *f.in.PhoneNumber != "+919800000001" {
		t.Errorf("published %+v", f.in)
	}
	if *f.in.MessageAttributes["AWS.SNS.SMS.SMSType"].StringValue != "Transactional" {
		t.Error("not transactional")
	}
}

type fakePushService struct {
	req    *http.Request
	status int
}

func (f *fakePushService) Do(req *http.Request) (*http.Response, error) {
	f.req = req
	return &http.Response{StatusCode: f.status, Status: http.StatusText(f.status), Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestWebPushPostsEncryptedPayloadToTheEndpoint(t *testing.T) {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	// a browser-side key pair for the subscription
	_, p256dh, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := json.Marshal(map[string]any{
		"endpoint": "https://push.example.test/abc",
		"keys":     map[string]string{"p256dh": p256dh, "auth": "dGVzdGF1dGhzZWNyZXQxMg"},
	})
	svc := &fakePushService{status: http.StatusCreated}
	w := &WebPush{PublicKey: pub, PrivateKey: priv, Subscriber: "mailto:t@example.test", Client: svc}
	if err := w.Send(context.Background(), alert.Message{Title: "t", Text: "x", Push: sub}); err != nil {
		t.Fatal(err)
	}
	if svc.req.URL.String() != "https://push.example.test/abc" || svc.req.Header.Get("Urgency") != "high" {
		t.Errorf("request %s %v", svc.req.URL, svc.req.Header)
	}

	svc.status = http.StatusGone // subscription expired
	if err := w.Send(context.Background(), alert.Message{Push: sub}); err == nil {
		t.Error("410 from the push service should be an error")
	}
	if err := w.Send(context.Background(), alert.Message{}); err == nil {
		t.Error("missing subscription should be an error")
	}
}
