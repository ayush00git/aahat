package assistant

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// EmergencyNumbers may always be said: they are fixed text, not data.
var EmergencyNumbers = []string{"112", "1077"}

// A number as written in prose: digits, optionally grouped with commas ("26,062", "10,85,800"),
// optionally a decimal part. A sign is not part of it ("-10.5" and "10.5" are the same figure to
// a reader). plainRE is the same without grouping, as JSON writes numbers.
var (
	numberRE = regexp.MustCompile(`(?:\d{1,3}(?:,\d{3})+|\d{1,2}(?:,\d{2})+,\d{3}|\d+)(?:\.\d+)?`)
	plainRE  = regexp.MustCompile(`\d+(?:\.\d+)?`)
)

var devanagariDigits = strings.NewReplacer(
	"०", "0", "१", "1", "२", "2", "३", "3", "४", "4", "५", "5", "६", "6", "७", "7", "८", "8", "९", "9")

// numbers lists the numbers in text in canonical form: Devanagari digits as ASCII, no grouping
// commas, no leading zeros, no trailing decimal zeros ("०५" -> "5", "1,085,800.0" -> "1085800").
func numbers(re *regexp.Regexp, text string) []string {
	var out []string
	for _, m := range re.FindAllString(devanagariDigits.Replace(text), -1) {
		out = append(out, canonical(m))
	}
	return out
}

func canonical(n string) string {
	n = strings.ReplaceAll(n, ",", "")
	whole, frac, _ := strings.Cut(n, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

// Allowed is the set of numbers an answer may contain.
type Allowed map[string]bool

// NewAllowed collects every number in the given source texts (tool results), the whole-number
// part of each (alerts round minutes down, so 19.6 may be said as 19), and the emergency numbers.
func NewAllowed(sources ...string) Allowed {
	a := Allowed{}
	for _, n := range EmergencyNumbers {
		a[n] = true
	}
	for _, src := range sources {
		// Both readings: "[605,123]" in JSON is two numbers, "26,062 m²" in a note is one. (So a
		// JSON list of small integers also admits its grouped reading; that is accepted.)
		for _, re := range []*regexp.Regexp{plainRE, numberRE} {
			for _, n := range numbers(re, src) {
				a[n] = true
				if f, err := strconv.ParseFloat(n, 64); err == nil && f < 1e15 {
					a[strconv.FormatFloat(math.Floor(f), 'f', -1, 64)] = true
				}
			}
		}
	}
	return a
}

// has reports whether a number as written in an answer is allowed: as one grouped number, or,
// for a list written without spaces ("2025,2026"), as each of its parts.
func (a Allowed) has(written string) bool {
	if a[canonical(written)] {
		return true
	}
	parts := strings.Split(written, ",")
	if len(parts) == 1 {
		return false
	}
	for _, p := range parts {
		if !a[canonical(p)] {
			return false
		}
	}
	return true
}

// Unsupported returns the numbers in answer that are not in the allowed set, in order of
// appearance and without repeats. An empty result means the answer passes.
func (a Allowed) Unsupported(answer string) []string {
	var bad []string
	seen := map[string]bool{}
	for _, n := range numberRE.FindAllString(devanagariDigits.Replace(answer), -1) {
		if !a.has(n) && !seen[n] {
			seen[n] = true
			bad = append(bad, n)
		}
	}
	return bad
}

// CallResults returns the result texts of the successful calls: the only source of numbers.
func CallResults(calls []Call) []string {
	var out []string
	for _, c := range calls {
		if !c.Failed {
			out = append(out, c.Result)
		}
	}
	return out
}
