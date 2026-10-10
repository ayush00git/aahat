package weather

import "fmt"

// Trigger levels, lowest first.
const (
	LevelNormal   = "normal"
	LevelElevated = "elevated"
	LevelHigh     = "high"
)

// The screening thresholds. They are our own choice, not a published standard: round numbers
// around the India Meteorological Department's "heavy rain" band (64.5 mm in a day), set lower
// because the lakes sit above most rain gauges and the forecast grid smooths mountain rain.
const (
	highDayMM       = 50.0  // any forecast day
	highTotalMM     = 100.0 // forecast days together
	elevatedDayMM   = 20.0
	elevatedTotalMM = 40.0
	heatJumpC       = 5.0 // forecast tmax above the mean tmax of the past days
)

// Rule is the rule Evaluate applies, stated in every response.
const Rule = "high if any forecast day has precipitation >= 50 mm, or the 3-day forecast total is >= 100 mm; " +
	"elevated if any forecast day has precipitation >= 20 mm, or the 3-day forecast total is >= 40 mm, " +
	"or a forecast day's maximum temperature is >= 5 C above the mean maximum temperature of the past 3 days (heat: fast melt); " +
	"otherwise normal. These are screening thresholds chosen by the Aahat team, not a published standard. " +
	"All values are from Open-Meteo."

type Trigger struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
	Rule    string   `json:"rule"`
}

// Evaluate applies Rule to an outlook. Missing values are ignored, so an outlook without
// data is "normal" with no reasons.
func Evaluate(days []Day) Trigger {
	var (
		wettest, hottest *Day
		total            float64
		pastSum          float64
		pastN            int
	)
	for i := range days {
		d := &days[i]
		if !d.Forecast {
			if d.TmaxC != nil {
				pastSum += *d.TmaxC
				pastN++
			}
			continue
		}
		if d.PrecipitationMM != nil {
			total += *d.PrecipitationMM
			if wettest == nil || *d.PrecipitationMM > *wettest.PrecipitationMM {
				wettest = d
			}
		}
		if d.TmaxC != nil && (hottest == nil || *d.TmaxC > *hottest.TmaxC) {
			hottest = d
		}
	}

	t := Trigger{Level: LevelNormal, Reasons: []string{}, Rule: Rule}
	raise := func(level, reason string) {
		if level == LevelHigh || t.Level == LevelNormal {
			t.Level = level
		}
		t.Reasons = append(t.Reasons, reason)
	}
	if wettest != nil {
		mm := *wettest.PrecipitationMM
		switch {
		case mm >= highDayMM:
			raise(LevelHigh, fmt.Sprintf("forecast precipitation of %.1f mm on %s (>= %.0f mm in a day)", mm, wettest.Date, highDayMM))
		case mm >= elevatedDayMM:
			raise(LevelElevated, fmt.Sprintf("forecast precipitation of %.1f mm on %s (>= %.0f mm in a day)", mm, wettest.Date, elevatedDayMM))
		}
	}
	switch {
	case total >= highTotalMM:
		raise(LevelHigh, fmt.Sprintf("forecast precipitation total of %.1f mm over the forecast days (>= %.0f mm)", total, highTotalMM))
	case total >= elevatedTotalMM:
		raise(LevelElevated, fmt.Sprintf("forecast precipitation total of %.1f mm over the forecast days (>= %.0f mm)", total, elevatedTotalMM))
	}
	if hottest != nil && pastN > 0 {
		mean := pastSum / float64(pastN)
		if jump := *hottest.TmaxC - mean; jump >= heatJumpC {
			raise(LevelElevated, fmt.Sprintf("forecast maximum temperature of %.1f C on %s is %.1f C above the mean of the past %d days (%.1f C): fast melt",
				*hottest.TmaxC, hottest.Date, jump, pastN, mean))
		}
	}
	return t
}
