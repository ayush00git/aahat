package data

import (
	"strings"
	"unicode"
)

// Place-name matching for GET /places/search. Villagers type a name the way they say it, so the
// same village arrives as "Sissu", "Sisu" or "सिस्सू". Names and queries are reduced to keys and
// the keys are compared; the keys of the places index are computed once, when it loads.
//
// LatinKey rules (deliberately few, so that different villages stay different):
//   - lowercase; Latin letters with a diacritic lose it (ā -> a, ṭ -> t);
//   - everything that is not a-z or 0-9 is dropped (spaces, hyphens, brackets, other scripts);
//   - w -> v ("Wangtu" = "Vangtu");
//   - a doubled letter counts once ("Sissu" = "Sisu", "Chhatru" = "Chatru", aa = a), except that
//     ee = i and oo = u ("Seema" = "Sima", "Koot" = "Kut"). Three or more of the same letter in a
//     row is not a spelling of anything and is left alone, so "zzz" does not turn into "z";
//   - ph -> f ("Phojal" = "Fojal").
//
// HindiKey keeps Devanagari letters only, without nukta, with chandrabindu read as anusvara.
//
// Two looser keys:
//   - BareKey: the LatinKey without the letter a, for names typed in Devanagari. Few places have a
//     Hindi name in the index, so the query is also transliterated letter by letter (Transliterate)
//     and compared with the English names. The transliteration has no inherent vowel, because
//     Hindi drops it unpredictably ("कोकसर" is Koksar, not Kokasara); dropping every a from both
//     sides makes them comparable. An equal BareKey counts as an exact match beside the Hindi
//     names; prefix and substring matches on it are used only when that finds nothing.
//   - Skeleton: the LatinKey without vowels, y and h, for spellings that differ only in vowels or
//     in an aspirate ("Kyelang" = "Keylong", "Khoksar" = "Koksar"). Compared for equality only,
//     only when it has at least three letters, and only when nothing else matched.

// SearchKeys are the precomputed keys of one name.
type SearchKeys struct {
	Latin    string
	Hindi    string
	Bare     string
	Skeleton string
}

// NewSearchKeys computes the keys of a place from its English and Hindi names.
func NewSearchKeys(name, nameHi string) SearchKeys {
	latin := LatinKey(name)
	return SearchKeys{Latin: latin, Hindi: HindiKey(nameHi), Bare: BareKey(latin), Skeleton: Skeleton(latin)}
}

// Latin letters with diacritics that occur in romanised Indian names, by base letter.
var diacritics = func() map[rune]rune {
	m := map[rune]rune{}
	for base, marked := range map[rune]string{
		'a': "āáàâäãå", 'e': "ēéèêë", 'i': "īíìîï", 'o': "ōóòôöõ", 'u': "ūúùûü",
		'c': "ç", 'd': "ḍ", 'h': "ḥ", 'l': "ḷ", 'm': "ṃṁ", 'n': "ñṅṇ", 'r': "ṛṝ", 's': "śṣš", 't': "ṭ", 'z': "ž",
	} {
		for _, r := range marked {
			m[r] = base
		}
	}
	return m
}()

// LatinKey reduces a name written in Latin letters to its search key (rules above).
func LatinKey(s string) string {
	letters := make([]byte, 0, len(s))
	for _, r := range strings.ToLower(s) {
		if base, ok := diacritics[r]; ok {
			r = base
		}
		switch {
		case r == 'w':
			letters = append(letters, 'v')
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			letters = append(letters, byte(r))
		}
	}
	out := make([]byte, 0, len(letters))
	for i := 0; i < len(letters); {
		c, n := letters[i], 1
		for i+n < len(letters) && letters[i+n] == c {
			n++
		}
		i += n
		digit := c >= '0' && c <= '9'
		switch {
		case n == 2 && c == 'e':
			out = append(out, 'i')
		case n == 2 && c == 'o':
			out = append(out, 'u')
		case n == 2 && !digit:
			out = append(out, c)
		default: // a single letter, digits ("Sector 11"), or a run of three or more
			for range n {
				out = append(out, c)
			}
		}
	}
	return strings.ReplaceAll(string(out), "ph", "f")
}

// BareKey drops every a from a LatinKey.
func BareKey(latinKey string) string { return strings.ReplaceAll(latinKey, "a", "") }

// Skeleton drops the vowels, y and h from a LatinKey.
func Skeleton(latinKey string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'a', 'e', 'i', 'o', 'u', 'y', 'h':
			return -1
		}
		return r
	}, latinKey)
}

const (
	nukta        = '़'
	virama       = '्'
	anusvara     = 'ं'
	chandrabindu = 'ँ'
)

// IsDevanagari reports whether r is in the Devanagari block.
func IsDevanagari(r rune) bool { return unicode.Is(unicode.Devanagari, r) }

// HasDevanagari reports whether s contains any Devanagari character.
func HasDevanagari(s string) bool { return strings.ContainsFunc(s, IsDevanagari) }

// Precomposed nukta letters, as their plain letter.
var nuktaLetters = map[rune]rune{
	'\u0958': 'क', '\u0959': 'ख', '\u095a': 'ग', '\u095b': 'ज',
	'\u095c': 'ड', '\u095d': 'ढ', '\u095e': 'फ', '\u095f': 'य',
}

// HindiKey reduces a name written in Devanagari to its search key.
func HindiKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if plain, ok := nuktaLetters[r]; ok {
			r = plain
		}
		switch {
		case r == nukta, !IsDevanagari(r):
		case r == chandrabindu:
			b.WriteRune(anusvara)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Devanagari to rough Latin. Consonants carry no inherent vowel (see BareKey).
var devanagariLatin = map[rune]string{
	'क': "k", 'ख': "kh", 'ग': "g", 'घ': "gh", 'ङ': "n",
	'च': "ch", 'छ': "chh", 'ज': "j", 'झ': "jh", 'ञ': "n",
	'ट': "t", 'ठ': "th", 'ड': "d", 'ढ': "dh", 'ण': "n",
	'त': "t", 'थ': "th", 'द': "d", 'ध': "dh", 'न': "n",
	'प': "p", 'फ': "ph", 'ब': "b", 'भ': "bh", 'म': "m",
	'य': "y", 'र': "r", 'ल': "l", 'ळ': "l", 'व': "v",
	'श': "sh", 'ष': "sh", 'स': "s", 'ह': "h",
	'अ': "a", 'आ': "a", 'इ': "i", 'ई': "i", 'उ': "u", 'ऊ': "u", 'ऋ': "ri",
	'ए': "e", 'ऐ': "ai", 'ओ': "o", 'औ': "au", 'ऑ': "o",
	'ा': "a", 'ि': "i", 'ी': "i", 'ु': "u", 'ू': "u", 'ृ': "ri",
	'े': "e", 'ै': "ai", 'ो': "o", 'ौ': "au", 'ॉ': "o",
	anusvara: "n", chandrabindu: "n",
	'०': "0", '१': "1", '२': "2", '३': "3", '४': "4", '५': "5", '६': "6", '७': "7", '८': "8", '९': "9",
}

// Transliterate writes Devanagari text in rough Latin letters: consonants and written vowels
// only. Virama, nukta, visarga and anything else are skipped. The result is meant for BareKey
// and Skeleton, not for display.
func Transliterate(s string) string {
	var b strings.Builder
	for _, r := range s {
		if plain, ok := nuktaLetters[r]; ok {
			r = plain
		}
		b.WriteString(devanagariLatin[r])
	}
	return b.String()
}

// Match tiers, best first. NoMatch is larger than all of them.
const (
	MatchExact = iota
	MatchPrefix
	MatchSubstring
	NoMatch
)

// MatchKey grades how a query key matches a name key.
func MatchKey(key, q string) int {
	switch {
	case q == "" || key == "":
		return NoMatch
	case key == q:
		return MatchExact
	case strings.HasPrefix(key, q):
		return MatchPrefix
	case strings.Contains(key, q):
		return MatchSubstring
	}
	return NoMatch
}

// minSkeleton is the shortest skeleton compared: with fewer consonants too many names are equal.
const minSkeleton = 3

// SearchQuery is a typed name, prepared once and then matched against many places.
type SearchQuery struct {
	passes []func(SearchKeys) int
}

// NewSearchQuery prepares q. Its passes are tried in order by the caller (Passes, Match): a later,
// looser pass is used only when the earlier ones found nothing in the whole index.
func NewSearchQuery(q string) SearchQuery {
	var sq SearchQuery
	add := func(f func(SearchKeys) int) { sq.passes = append(sq.passes, f) }
	skeleton := func(latin string) {
		if sk := Skeleton(latin); len(sk) >= minSkeleton {
			add(func(k SearchKeys) int {
				if k.Skeleton == sk {
					return MatchSubstring
				}
				return NoMatch
			})
		}
	}
	if HasDevanagari(q) {
		// Few places have a Hindi name in the index, so the first pass also takes the places whose
		// English name is the same word; names that merely start with or contain it come second.
		hi := HindiKey(q)
		latin := LatinKey(Transliterate(q))
		bare := BareKey(latin)
		if len(bare) < 2 {
			bare = ""
		}
		add(func(k SearchKeys) int {
			if bare != "" && k.Bare == bare {
				return MatchExact
			}
			return MatchKey(k.Hindi, hi)
		})
		if bare != "" {
			add(func(k SearchKeys) int { return MatchKey(k.Bare, bare) })
		}
		skeleton(latin)
		return sq
	}
	latin := LatinKey(q)
	if latin != "" {
		add(func(k SearchKeys) int { return MatchKey(k.Latin, latin) })
		skeleton(latin)
	}
	return sq
}

// Passes is the number of passes to try.
func (q SearchQuery) Passes() int { return len(q.passes) }

// Match grades a place's keys in one pass.
func (q SearchQuery) Match(pass int, k SearchKeys) int { return q.passes[pass](k) }

// PlaceSize ranks an OSM place kind, larger places first; unknown kinds go last.
func PlaceSize(kind string) int {
	switch kind {
	case "city":
		return 0
	case "town":
		return 1
	case "suburb":
		return 2
	case "village":
		return 3
	case "hamlet":
		return 4
	}
	return 5
}
