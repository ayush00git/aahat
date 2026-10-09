package voice

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
)

type fakePolly struct {
	calls int
	lang  types.LanguageCode
}

func (f *fakePolly) SynthesizeSpeech(_ context.Context, in *polly.SynthesizeSpeechInput, _ ...func(*polly.Options)) (*polly.SynthesizeSpeechOutput, error) {
	f.calls++
	f.lang = in.LanguageCode
	return &polly.SynthesizeSpeechOutput{AudioStream: io.NopCloser(strings.NewReader("ID3fake"))}, nil
}

func TestAudioIsSynthesizedOnceAndServedByName(t *testing.T) {
	f := &fakePolly{}
	p, err := NewPolly(f, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u1, err := p.AudioURL(context.Background(), "चेतावनी", "hi")
	if err != nil {
		t.Fatal(err)
	}
	u2, _ := p.AudioURL(context.Background(), "चेतावनी", "hi")
	if u1 != u2 || f.calls != 1 || f.lang != types.LanguageCodeHiIn {
		t.Errorf("u1=%s u2=%s calls=%d lang=%s", u1, u2, f.calls, f.lang)
	}
	name := strings.TrimPrefix(u1, "/audio/")
	if b, err := os.ReadFile(p.Path(name)); err != nil || string(b) != "ID3fake" {
		t.Errorf("file: %v %q", err, b)
	}
	if p.Path("../../etc/passwd") != "" || p.Path("x.mp3") != "" {
		t.Error("Path must reject names that are not content hashes")
	}
}
