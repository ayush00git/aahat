package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ayush00git/aahat/api/internal/assistant"
	"github.com/ayush00git/aahat/api/internal/data"
)

// ErrUnresolved: the lake's position could not be found by name. Its message is shown to the caller.
var ErrUnresolved = errors.New("could not find this water body by name; send its coordinates (lat, lon) with the request")

// Resolver finds where a named water body is.
type Resolver interface {
	Resolve(ctx context.Context, name string) (*Lake, error)
}

// candidate is one search hit: a monitored lake or a named settlement.
type candidate struct {
	Ref      string  `json:"ref"`
	Kind     string  `json:"kind"` // monitored_lake | settlement
	Name     string  `json:"name"`
	NameHi   string  `json:"name_hi,omitempty"`
	District string  `json:"district,omitempty"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	source   string
}

const maxCandidates = 10

// Index searches the monitored lakes and the region's place index by name.
type Index struct {
	Catalog *data.Catalog
}

// fold lowercases a name and drops spaces and punctuation, so "Chandra Tal", "chandratal" and
// "Chandra-Tal" compare equal.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '-', '_', '.', ',', '\'', '’':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stem drops a trailing word for "lake", so "Bhrigu Lake" finds "Bhrigu".
func stem(folded string) string {
	for _, suffix := range []string{"lake", "झील", "tal", "ताल", "tso", "dal"} {
		if rest, ok := strings.CutSuffix(folded, suffix); ok && len(rest) >= 4 {
			return rest
		}
	}
	return folded
}

// lakes returns the monitored lakes matching name: exact (folded) matches only, or any that
// contain the name's stem.
func (x Index) lakes(ctx context.Context, name string, exact bool) ([]candidate, error) {
	idx, err := x.Catalog.Index(ctx)
	if errors.Is(err, data.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q := fold(name)
	var out []candidate
	for _, l := range idx.Lakes {
		var pos struct {
			Lat      *float64 `json:"lat"`
			Lon      *float64 `json:"lon"`
			District string   `json:"district"`
		}
		if json.Unmarshal(l.Raw, &pos) != nil || pos.Lat == nil || pos.Lon == nil {
			continue
		}
		names := []string{fold(l.Name), fold(l.NameHi), fold(l.ID)}
		match := false
		for _, n := range names {
			if n == "" {
				continue
			}
			if exact {
				match = match || n == q
			} else {
				match = match || strings.Contains(n, stem(q)) || strings.Contains(q, stem(n))
			}
		}
		if match {
			out = append(out, candidate{Kind: "monitored_lake", Name: l.Name, NameHi: l.NameHi, District: pos.District,
				Lat: *pos.Lat, Lon: *pos.Lon, source: "catalogue:" + l.ID})
		}
	}
	return out, nil
}

// search returns monitored lakes first, then settlements whose name starts with (then contains)
// the name's stem.
func (x Index) search(ctx context.Context, name string) ([]candidate, error) {
	q := stem(fold(name))
	if len([]rune(q)) < 3 {
		return nil, errors.New("name too short to search (3 letters at least)")
	}
	out, err := x.lakes(ctx, name, false)
	if err != nil {
		return nil, err
	}
	places, err := x.Catalog.Places(ctx)
	if err != nil {
		return nil, err
	}
	var prefix, inside []candidate
	for _, p := range places {
		n, nh := fold(p.Name), fold(data.Str(p.NameHi))
		c := candidate{Kind: "settlement", Name: p.Name, NameHi: data.Str(p.NameHi), Lat: p.Lat, Lon: p.Lon, source: "places:" + p.OSM}
		switch {
		case strings.HasPrefix(n, q) || (nh != "" && strings.HasPrefix(nh, q)):
			prefix = append(prefix, c)
		case strings.Contains(n, q) || (nh != "" && strings.Contains(nh, q)):
			inside = append(inside, c)
		}
	}
	out = append(append(out, prefix...), inside...)
	return out[:min(len(out), maxCandidates)], nil
}

// CatalogueResolver resolves only names that are exactly one monitored lake. It needs no model.
type CatalogueResolver struct {
	Index Index
}

func (r CatalogueResolver) Resolve(ctx context.Context, name string) (*Lake, error) {
	found, err := r.Index.lakes(ctx, name, true)
	if err != nil {
		return nil, err
	}
	if len(found) != 1 {
		return nil, ErrUnresolved
	}
	return &Lake{Name: found[0].Name, Lat: found[0].Lat, Lon: found[0].Lon, Source: found[0].source}, nil
}

// ClaudeResolver lets the model search by name (spelling variants, Hindi) and pick one of the
// hits. The model never states coordinates: it can only point at a search result by its ref, and
// the position is read from that result.
type ClaudeResolver struct {
	Client assistant.Converser
	Model  string
	Index  Index
}

const resolvePrompt = `You help officials of Aahat, a glacial lake flood watch for Himachal Pradesh, India, locate a lake or other water body they want analysed from satellite images. They give only a name, possibly misspelled, transliterated differently (Chandratal / Chandra Tal / चंद्रताल) or with a word like "lake", "tal", "jheel" or "tso" attached.

Use find_water to search the monitored lakes and the region's index of named settlements. Try other spellings or the bare name if the first search finds nothing useful (at most 4 searches).

Then finish with exactly one tool call:
- choose(ref) with the ref of the search result that is this water body. A monitored lake of the same name is the best match. A settlement may be chosen only when the water body is clearly named after it and lies at it (the analysis looks for water within 1.5 km of the chosen point, so a village that merely shares a common name with the lake is the wrong choice).
- give_up(reason) if no result is clearly this water body, or several are equally likely. The official will then be asked for coordinates, which is always better than analysing the wrong place.

You cannot supply coordinates yourself, and you must not guess: only a ref from a find_water result counts.`

var resolveTools = []assistant.Tool{
	{
		Name:        "find_water",
		Description: "Search the monitored lakes and the named settlements of the region by name. Returns up to 10 results, each with a ref, kind (monitored_lake or settlement), names and position.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string", "description": "Name or part of a name, English or Hindi"}},
			"required":   []string{"name"},
		},
	},
	{
		Name:        "choose",
		Description: "Finish: this search result is the water body asked for.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"ref": map[string]any{"type": "string", "description": "The ref of a find_water result, like r3"}},
			"required":   []string{"ref"},
		},
	},
	{
		Name:        "give_up",
		Description: "Finish: no search result is clearly the water body asked for.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"reason": map[string]any{"type": "string"}},
			"required":   []string{"reason"},
		},
	},
}

func (r ClaudeResolver) Resolve(ctx context.Context, name string) (*Lake, error) {
	// A name that is exactly one monitored lake needs no model.
	if lake, err := (CatalogueResolver{Index: r.Index}).Resolve(ctx, name); err == nil {
		return lake, nil
	}
	seen := map[string]candidate{}
	var chosen *candidate
	gaveUp := false
	exec := func(ctx context.Context, tool string, args map[string]any) (string, error) {
		switch tool {
		case "find_water":
			found, err := r.Index.search(ctx, assistant.StringArg(args, "name"))
			if err != nil {
				return "", err
			}
			for i := range found {
				found[i].Ref = fmt.Sprintf("r%d", len(seen)+1)
				seen[found[i].Ref] = found[i]
			}
			if found == nil {
				found = []candidate{}
			}
			b, _ := json.Marshal(map[string]any{"results": found})
			return string(b), nil
		case "choose":
			c, ok := seen[assistant.StringArg(args, "ref")]
			if !ok {
				return "", errors.New("no find_water result has this ref")
			}
			chosen = &c
			return "recorded", nil
		case "give_up":
			gaveUp = true
			return "recorded", nil
		}
		return "", fmt.Errorf("unknown tool %q", tool)
	}
	conv := assistant.NewConversation(resolvePrompt, resolveTools, "Water body to locate: "+name)
	runner := assistant.Runner{Client: r.Client, Model: r.Model, MaxTokens: 300, MaxRounds: 6}
	_, err := runner.Run(ctx, conv, exec)
	switch {
	case chosen != nil && !gaveUp:
		return &Lake{Name: chosen.Name, Lat: chosen.Lat, Lon: chosen.Lon, Source: chosen.source}, nil
	case err != nil && !gaveUp && !errors.Is(err, assistant.ErrTooManyRounds):
		return nil, err
	}
	return nil, ErrUnresolved
}
