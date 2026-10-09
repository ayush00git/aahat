// Package voice reads warnings aloud with Amazon Polly and keeps the MP3s on disk, so the
// villager app can play them and a voice channel can use them.
package voice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
)

// Kajal (neural) speaks Hindi and Indian English; the same voice for both keeps alerts consistent.
var languages = map[string]types.LanguageCode{
	"hi": types.LanguageCodeHiIn,
	"en": types.LanguageCodeEnIn,
}

// Synthesizer is the part of the Polly client used here (a fake in tests).
type Synthesizer interface {
	SynthesizeSpeech(ctx context.Context, in *polly.SynthesizeSpeechInput, opts ...func(*polly.Options)) (*polly.SynthesizeSpeechOutput, error)
}

// Polly synthesizes each distinct (language, text) once and serves it from Dir as /audio/<hash>.mp3.
type Polly struct {
	Client Synthesizer
	Dir    string
	Voice  types.VoiceId
	Engine types.Engine
}

func NewPolly(client Synthesizer, dir string) (*Polly, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Polly{Client: client, Dir: dir, Voice: types.VoiceIdKajal, Engine: types.EngineNeural}, nil
}

// FileName is the cache key for a spoken message.
func (p *Polly) FileName(text, lang string) string {
	h := sha256.Sum256([]byte(lang + "|" + string(p.Voice) + "|" + string(p.Engine) + "|" + text))
	return hex.EncodeToString(h[:8]) + ".mp3"
}

func (p *Polly) AudioURL(ctx context.Context, text, lang string) (string, error) {
	name := p.FileName(text, lang)
	path := filepath.Join(p.Dir, name)
	if _, err := os.Stat(path); err == nil {
		return "/audio/" + name, nil
	}
	code, ok := languages[lang]
	if !ok {
		code = types.LanguageCodeHiIn
	}
	out, err := p.Client.SynthesizeSpeech(ctx, &polly.SynthesizeSpeechInput{
		Text:         aws.String(text),
		VoiceId:      p.Voice,
		Engine:       p.Engine,
		LanguageCode: code,
		OutputFormat: types.OutputFormatMp3,
	})
	if err != nil {
		return "", fmt.Errorf("polly: %w", err)
	}
	defer out.AudioStream.Close()
	tmp, err := os.CreateTemp(p.Dir, "tmp-*.mp3")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, out.AudioStream); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return "/audio/" + name, nil
}

var audioName = regexp.MustCompile(`^[0-9a-f]{16}\.mp3$`)

// Path returns the file for an /audio/{name} request, or "" if the name is not one of ours.
func (p *Polly) Path(name string) string {
	if !audioName.MatchString(name) {
		return ""
	}
	return filepath.Join(p.Dir, name)
}
