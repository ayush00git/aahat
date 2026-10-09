package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/notify"
	"github.com/ayush00git/aahat/api/internal/voice"
)

// Channels sets up delivery from environment variables; anything not configured logs instead:
//
//	AAHAT_POLLY=1                          speak warnings with Polly (Kajal, hi-IN/en-IN) into AAHAT_AUDIO_DIR
//	AAHAT_AUDIO_DIR                        where MP3s are kept (default <stateDir>/audio)
//	AAHAT_VAPID_PUBLIC_KEY/_PRIVATE_KEY    web push keys; if unset they are generated once into <stateDir>/vapid.json
//	AAHAT_PUSH_SUBJECT                     contact for push services (default mailto:alerts@aahat.app)
//	AAHAT_SMS=sns                          send SMS through Amazon SNS (sandbox: verified numbers only)
//
// Voice calls stay log-only: AWS voice routes to India need a support case to open, and the
// voice-message API has no Hindi voice.
func Channels(ctx context.Context, log *slog.Logger, stateDir string) (alert.Notifier, *voice.Polly, string, error) {
	logOnly := alert.NewLogNotifier(log)
	router := &notify.Router{ByChannel: map[string]alert.Notifier{}, Fallback: logOnly}

	pub, priv, err := vapidKeys(stateDir)
	if err != nil {
		return nil, nil, "", err
	}
	subject := os.Getenv("AAHAT_PUSH_SUBJECT")
	if subject == "" {
		subject = "mailto:alerts@aahat.app"
	}
	router.ByChannel[alert.ChannelWebPush] = &notify.WebPush{PublicKey: pub, PrivateKey: priv, Subscriber: subject}

	var speaker *voice.Polly
	if os.Getenv("AAHAT_POLLY") == "1" || os.Getenv("AAHAT_SMS") == "sns" {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, nil, "", fmt.Errorf("load AWS config: %w", err)
		}
		if os.Getenv("AAHAT_POLLY") == "1" {
			dir := os.Getenv("AAHAT_AUDIO_DIR")
			if dir == "" {
				dir = filepath.Join(stateDir, "audio")
			}
			if speaker, err = voice.NewPolly(polly.NewFromConfig(cfg), dir); err != nil {
				return nil, nil, "", err
			}
		}
		if os.Getenv("AAHAT_SMS") == "sns" {
			router.ByChannel[alert.ChannelSMS] = &notify.SNSSMS{Client: sns.NewFromConfig(cfg)}
		}
	}
	return router, speaker, pub, nil
}

// vapidKeys reads the web push key pair from the environment, or from (and on first use into)
// <stateDir>/vapid.json, so browser subscriptions survive restarts.
func vapidKeys(stateDir string) (string, string, error) {
	if pub, priv := os.Getenv("AAHAT_VAPID_PUBLIC_KEY"), os.Getenv("AAHAT_VAPID_PRIVATE_KEY"); pub != "" && priv != "" {
		return pub, priv, nil
	}
	path := filepath.Join(stateDir, "vapid.json")
	var keys struct{ Public, Private string }
	b, err := os.ReadFile(path)
	if err == nil && json.Unmarshal(b, &keys) == nil && keys.Public != "" {
		return keys.Public, keys.Private, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	keys.Public, keys.Private = pub, priv
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", "", err
	}
	b, _ = json.Marshal(keys)
	return pub, priv, os.WriteFile(path, b, 0o600)
}
