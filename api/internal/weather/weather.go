// Package weather fetches a short precipitation and temperature outlook for a lake from
// Open-Meteo and turns it into a simple screening level ("trigger"). Heavy rain and sudden
// heat (fast melt) are the usual weather triggers of a glacial lake outburst.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// Source is the attribution carried in every response.
	Source = "Open-Meteo (open-meteo.com)"
	// DefaultURL is Open-Meteo's forecast endpoint (free, no key).
	DefaultURL = "https://api.open-meteo.com/v1/forecast"

	// The outlook is the past PastDays days followed by ForecastDays days starting today.
	PastDays     = 3
	ForecastDays = 3
)

// Day is one day of the outlook. Values are pointers because Open-Meteo sends null where it
// has no value, and a missing value must not become a zero.
type Day struct {
	Date            string   `json:"date"`
	PrecipitationMM *float64 `json:"precipitation_mm"`
	SnowfallCM      *float64 `json:"snowfall_cm"`
	TmaxC           *float64 `json:"tmax_c"`
	TminC           *float64 `json:"tmin_c"`
	// Forecast is false for the past days and true from today on.
	Forecast bool `json:"forecast"`
}

// Fetcher returns the outlook for a point, oldest day first.
type Fetcher interface {
	Fetch(ctx context.Context, lat, lon float64) ([]Day, error)
}

// Client is the Open-Meteo Fetcher.
type Client struct {
	URL  string
	HTTP *http.Client
}

// NewClient returns a client for baseURL, or for DefaultURL if it is empty.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{URL: baseURL, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) Fetch(ctx context.Context, lat, lon float64) ([]Day, error) {
	q := url.Values{
		"latitude":      {strconv.FormatFloat(lat, 'f', -1, 64)},
		"longitude":     {strconv.FormatFloat(lon, 'f', -1, 64)},
		"daily":         {"precipitation_sum,temperature_2m_max,temperature_2m_min,snowfall_sum"},
		"past_days":     {strconv.Itoa(PastDays)},
		"forecast_days": {strconv.Itoa(ForecastDays)},
		"timezone":      {"Asia/Kolkata"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo: status %d: %.200s", resp.StatusCode, body)
	}
	return parse(body)
}

func parse(body []byte) ([]Day, error) {
	var doc struct {
		Daily struct {
			Time   []string   `json:"time"`
			Precip []*float64 `json:"precipitation_sum"`
			Tmax   []*float64 `json:"temperature_2m_max"`
			Tmin   []*float64 `json:"temperature_2m_min"`
			Snow   []*float64 `json:"snowfall_sum"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	d := doc.Daily
	if len(d.Time) == 0 {
		return nil, errors.New("open-meteo: no daily values")
	}
	at := func(vs []*float64, i int) *float64 {
		if i < len(vs) {
			return vs[i]
		}
		return nil
	}
	days := make([]Day, len(d.Time))
	for i, date := range d.Time {
		days[i] = Day{
			Date: date, PrecipitationMM: at(d.Precip, i), SnowfallCM: at(d.Snow, i),
			TmaxC: at(d.Tmax, i), TminC: at(d.Tmin, i),
			Forecast: i >= PastDays, // the request asks for PastDays past days first
		}
	}
	return days, nil
}
