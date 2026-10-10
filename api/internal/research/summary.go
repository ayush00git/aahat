package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ayush00git/aahat/api/internal/assistant"
)

// Summarizer writes a short plain-language reading of a finished job's results. mode is
// assistant.ModeModel or assistant.ModeTemplate.
type Summarizer interface {
	Summarize(ctx context.Context, job *Job) (text, mode string, err error)
}

// results is the part of the worker's results a summary is built from.
type results struct {
	Snapped *struct {
		Lat     float64  `json:"lat"`
		Lon     float64  `json:"lon"`
		AreaKm2 *float64 `json:"area_km2"`
		ElevM   *float64 `json:"elev_m"`
	} `json:"snapped"`
	Series []struct {
		Year   int      `json:"year"`
		Status string   `json:"status"`
		AreaM2 *float64 `json:"area_m2"`
	} `json:"series"`
	Risk *struct {
		Score     *float64 `json:"score"`
		Level     string   `json:"level"`
		DataUntil string   `json:"data_until"`
	} `json:"risk"`
}

// TemplateSummary states the measured figures with no model involved.
func TemplateSummary(job *Job) string {
	var r results
	if len(job.Results) == 0 || json.Unmarshal(job.Results, &r) != nil {
		return ""
	}
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	var measured []string
	for _, y := range r.Series {
		if y.AreaM2 != nil {
			s := fmt.Sprintf("%d: %s m²", y.Year, num(*y.AreaM2))
			if y.Status != "ok" {
				s += " (" + y.Status + ")"
			}
			measured = append(measured, s)
		}
	}
	name := job.Name
	if job.Lake != nil && job.Lake.Name != "" {
		name = job.Lake.Name
	}
	var b strings.Builder
	if len(measured) == 0 {
		fmt.Fprintf(&b, "%s: no lake outline could be measured in the seasons %d-%d.", name, job.YearFrom, job.YearTo)
	} else {
		fmt.Fprintf(&b, "%s: post-monsoon lake area by season: %s.", name, strings.Join(measured, "; "))
	}
	if r.Risk != nil && r.Risk.Score != nil {
		fmt.Fprintf(&b, " Screening hazard score %s (%s)", num(*r.Risk.Score), strings.ReplaceAll(r.Risk.Level, "_", " "))
		if r.Risk.DataUntil != "" {
			b.WriteString(", data until " + r.Risk.DataUntil)
		}
		b.WriteString(". The score ranks lakes for attention; it is not a probability of failure.")
	}
	return b.String()
}

// ClaudeSummarizer asks the model for the summary and checks every number in it against the
// job's data, as /ask does; after two failed checks it falls back to TemplateSummary.
type ClaudeSummarizer struct {
	Client assistant.Converser
	Model  string
}

const summaryPrompt = `You write short summaries for disaster-management officials in Himachal Pradesh, India, who asked Aahat (a glacial lake flood watch) to analyse a lake from Sentinel-2 satellite images. They are not remote-sensing specialists. You are given the job's data as JSON: the lake, the yearly post-monsoon water area ("series": area_m2 with its uncertainty_m2; status "ok" means the lake was fully seen, "partial" means clouds or too few scenes so that year is unreliable, "not_found"/"no_data" mean no measurement) and, if computed, a screening hazard score out of 100 with its factors ("risk").

Write 3 to 5 plain English sentences, no markdown or lists: what was measured and over which seasons, whether the area is growing, shrinking or steady (compare areas only where the difference is larger than the uncertainties; otherwise say it is steady within measurement uncertainty), the hazard score and level with the main factor behind it, and any caveat the data shows (partial years, few seasons, no score).

Numbers: every number you write must appear in the JSON, copied exactly. Do not calculate differences, percentages or unit conversions, and do not round. Say "grew" or "shrank" in words instead of computing by how much. The score is a screening score for prioritising attention, not a probability; never say or hint when or whether the lake will burst.`

func (s ClaudeSummarizer) Summarize(ctx context.Context, job *Job) (string, string, error) {
	facts, err := json.Marshal(map[string]any{
		"lake": job.Lake, "requested_name": job.Name, "year_from": job.YearFrom, "year_to": job.YearTo,
		"results": summaryFacts(job.Results), "score_out_of": 100,
	})
	if err != nil {
		return "", "", err
	}
	allowed := assistant.NewAllowed(string(facts))
	conv := assistant.NewConversation(summaryPrompt, nil, string(facts))
	runner := assistant.Runner{Client: s.Client, Model: s.Model, MaxTokens: 400, MaxRounds: 1}
	noTools := func(context.Context, string, map[string]any) (string, error) { return "", fmt.Errorf("no tools here") }

	text, err := runner.Run(ctx, conv, noTools)
	if err != nil {
		return "", "", err
	}
	bad := allowed.Unsupported(text)
	if len(bad) > 0 || text == "" {
		conv.Say("Your summary was not accepted: these numbers are not in the JSON: " + strings.Join(bad, ", ") +
			". Write it again using only numbers copied exactly from the JSON; describe changes in words instead of computing them.")
		if text, err = runner.Run(ctx, conv, noTools); err != nil {
			return "", "", err
		}
		bad = allowed.Unsupported(text)
	}
	if len(bad) > 0 || text == "" {
		return TemplateSummary(job), assistant.ModeTemplate, nil
	}
	return text, assistant.ModeModel, nil
}

// summaryFacts drops what a summary does not need (file lists, paths) from the worker's results.
func summaryFacts(raw json.RawMessage) map[string]any {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range []string{"snapped", "series", "risk"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}
