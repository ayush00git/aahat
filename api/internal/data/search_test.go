package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestLatinKey(t *testing.T) {
	for in, want := range map[string]string{
		"Sissu":       "sisu",
		"Sisu":        "sisu",
		"Chhatru":     "chatru",
		"Tāndī":       "tandi",
		"Seema":       "sima",
		"Koot":        "kut",
		"Baag":        "bag",
		"Wangtu":      "vangtu",
		"Phojal":      "fojal",
		"Old Manali":  "oldmanali",
		"Kaza (Kaja)": "kazakaja",
		"Sector 11":   "sector11",
		"zzz":         "zzz",
		"केलांग":      "",
	} {
		if got := LatinKey(in); got != want {
			t.Errorf("LatinKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTransliterate(t *testing.T) {
	for in, want := range map[string]string{
		"सिस्सू":  "sissu",
		"मनाली":   "mnali",
		"कोकसर":   "koksr",
		"केलांग":  "kelang",
		"काज़ा":   "kaja",
		"छतड़ू":   "chhtdu",
		"हमीरपुर": "hmirpur",
	} {
		if got := Transliterate(in); got != want {
			t.Errorf("Transliterate(%q) = %q, want %q", in, got, want)
		}
	}
	// What matters: a name typed in Hindi gets the same bare key as its usual English spelling.
	for hi, en := range map[string]string{"सिस्सू": "Sissu", "मनाली": "Manali", "कोकसर": "Koksar", "हमीरपुर": "Hamirpur", "रामपुर": "Rampur"} {
		if a, b := BareKey(LatinKey(Transliterate(hi))), BareKey(LatinKey(en)); a != b {
			t.Errorf("%s -> %q, %s -> %q", hi, a, en, b)
		}
	}
	if a, b := Skeleton(LatinKey("Kyelang")), Skeleton(LatinKey("Keylong")); a != b || a != "klng" {
		t.Errorf("skeletons %q, %q", a, b)
	}
}

func TestHindiKey(t *testing.T) {
	if a, b := HindiKey("काज़ा "), HindiKey("काजा"); a != b {
		t.Errorf("nukta: %q != %q", a, b)
	}
	if a, b := HindiKey("चाँद पुर"), HindiKey("चांदपुर"); a != b {
		t.Errorf("chandrabindu and space: %q != %q", a, b)
	}
}

// benchStore serves a places index of n settlements.
type benchStore struct{ body []byte }

func (b benchStore) Get(_ context.Context, key string) ([]byte, error) {
	if key == placesKey {
		return b.body, nil
	}
	return nil, ErrNotFound
}

func benchIndex(n int) []byte {
	syll := []string{"ma", "na", "li", "ke", "long", "si", "su", "ko", "sar", "tan", "di", "pur", "kha", "ri", "cha", "tru"}
	var b strings.Builder
	b.WriteString(`{"places":[`)
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		name := syll[i%16] + syll[(i/16)%16] + syll[(i/256)%16]
		fmt.Fprintf(&b, `{"osm":"node/%d","name":%q,"name_hi":null,"place":"hamlet","lon":77,"lat":32}`, i+1, name)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

// One search is a single pass over the precomputed keys of the index (about 19,800 places in
// production), plus a second pass when a looser match is needed.
func BenchmarkSearch20k(b *testing.B) {
	c := NewCatalog(benchStore{benchIndex(20000)})
	places, err := c.Places(context.Background())
	if err != nil || len(places) != 20000 {
		b.Fatal(len(places), err)
	}
	for _, q := range []string{"Sisu", "सिस्सू", "Kyelang", "qwxzkj"} {
		b.Run(q, func(b *testing.B) {
			for b.Loop() {
				query := NewSearchQuery(q)
				found := 0
				for pass := 0; pass < query.Passes() && found == 0; pass++ {
					for i := range places {
						if query.Match(pass, places[i].Keys) != NoMatch {
							found++
						}
					}
				}
			}
		})
	}
}

func TestPlacesKeysComputedOnce(t *testing.T) {
	c := NewCatalog(benchStore{benchIndex(3)})
	a, _ := c.Places(context.Background())
	b, _ := c.Places(context.Background())
	if len(a) != 3 || &a[0] != &b[0] || a[0].Keys.Latin != "mamama" || a[0].Keys.Skeleton != "mmm" {
		t.Fatalf("places = %+v", a)
	}
}
