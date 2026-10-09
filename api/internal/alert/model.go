// Package alert turns a lake trigger into a warning plan (who to warn, in
// which order) and sends it through a Notifier.
package alert

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ErrInvalid marks errors caused by bad input; the HTTP layer maps it to 400.
var ErrInvalid = errors.New("invalid request")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

const (
	LangHindi   = "hi"
	LangEnglish = "en"

	ChannelSMS      = "sms"
	ChannelVoice    = "voice"
	ChannelWhatsApp = "whatsapp"
	ChannelWebPush  = "webpush" // browser push to the villager web app

	SourceSimulation = "simulation"
	SourceSensor     = "sensor"

	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"
)

// Subscription asks for warnings about one place (an OSM settlement, school
// or health facility) to be sent to one phone.
type Subscription struct {
	ID        string `json:"id"`
	PlaceOSM  string `json:"place_osm"`
	PlaceName string `json:"place_name"`
	Phone     string `json:"phone"`
	Lang      string `json:"lang"`
	Channel   string `json:"channel"`
	// Push is the browser's PushSubscription JSON for the webpush channel.
	Push      json.RawMessage `json:"push_subscription,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

var (
	e164   = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	osmRef = regexp.MustCompile(`^(node|way|relation)/[0-9]+$`)
)

// ValidOSM reports whether s looks like "node/123", "way/123" or "relation/123".
func ValidOSM(s string) bool { return osmRef.MatchString(s) }

// Normalize fills defaults and validates the fields a caller supplies.
func (s *Subscription) Normalize() error {
	if !ValidOSM(s.PlaceOSM) {
		return invalid("place_osm must look like node/123, way/123 or relation/123")
	}
	if s.Channel == "" {
		s.Channel = ChannelSMS
	}
	if s.Channel == ChannelWebPush {
		var push struct {
			Endpoint string `json:"endpoint"`
		}
		if json.Unmarshal(s.Push, &push) != nil || push.Endpoint == "" {
			return invalid("push_subscription with an endpoint is required for the webpush channel")
		}
	} else {
		s.Push = nil
	}
	// A phone number is needed for SMS, voice and WhatsApp; optional for push.
	if (s.Channel != ChannelWebPush || s.Phone != "") && !e164.MatchString(s.Phone) {
		return invalid("phone must be in E.164 format, e.g. +919812345678")
	}
	if len(s.PlaceName) > 200 {
		return invalid("place_name is too long")
	}
	if s.Lang == "" {
		s.Lang = LangHindi
	}
	if s.Lang != LangHindi && s.Lang != LangEnglish {
		return invalid(`lang must be "hi" or "en"`)
	}
	switch s.Channel {
	case ChannelSMS, ChannelVoice, ChannelWhatsApp, ChannelWebPush:
	default:
		return invalid(`channel must be "sms", "voice", "whatsapp" or "webpush"`)
	}
	return nil
}

// SensorReading is the payload of the sensor webhook, kept on the event.
type SensorReading struct {
	LakeID     string    `json:"lake_id"`
	SensorID   string    `json:"sensor_id"`
	Kind       string    `json:"kind"`
	Value      *float64  `json:"value"`
	ObservedAt time.Time `json:"observed_at"`
}

// Event is one entry in the alert log: a trigger, the plan built from it and
// what happened to each message.
type Event struct {
	ID             string          `json:"event_id"`
	LakeID         string          `json:"lake_id"`
	LakeName       string          `json:"lake_name"`
	LakeNameHi     string          `json:"lake_name_hi"`
	Scenario       string          `json:"scenario"`
	Source         string          `json:"source"`
	Note           string          `json:"note,omitempty"`
	Sensor         *SensorReading  `json:"sensor,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	Recipients     []Recipient     `json:"recipients"`
	AffectedPlaces []AffectedPlace `json:"affected_places"`
	Summary        Summary         `json:"summary"`
}

// Recipient is one message in the fan-out plan.
type Recipient struct {
	SubscriptionID     string   `json:"subscription_id"`
	Phone              string   `json:"phone"`
	Lang               string   `json:"lang"`
	Channel            string   `json:"channel"`
	PlaceOSM           string   `json:"place_osm"`
	PlaceName          string   `json:"place_name"`
	ArrivalMinFast     *float64 `json:"arrival_min_fast"`
	ArrivalMinExpected *float64 `json:"arrival_min_expected"`
	// Status is the place's overall status from impacts.json.
	Status  string `json:"status"`
	Message string `json:"message"`
	Short   string `json:"short_message,omitempty"` // SMS-length version
	// AudioURL is the message read out in the recipient's language (Polly), relative to the API root.
	AudioURL string   `json:"audio_url,omitempty"`
	Delivery Delivery `json:"delivery"`
	// push carries the browser subscription to the dispatcher; never stored or returned.
	push json.RawMessage
}

type Delivery struct {
	Status string     `json:"status"`
	Error  string     `json:"error,omitempty"`
	At     *time.Time `json:"at,omitempty"`
}

// AffectedPlace is a settlement, school or health facility hit under the
// triggered scenario, whether or not anyone subscribed to it.
type AffectedPlace struct {
	OSM                string   `json:"osm"`
	Name               *string  `json:"name"`
	NameHi             *string  `json:"name_hi"`
	Kind               string   `json:"kind"`
	Subkind            string   `json:"subkind"`
	Lon                float64  `json:"lon"`
	Lat                float64  `json:"lat"`
	Km                 *float64 `json:"km"`
	ArrivalMinFast     *float64 `json:"arrival_min_fast"`
	ArrivalMinExpected *float64 `json:"arrival_min_expected"`
	Status             string   `json:"status"`
	// ScenarioStatus, FloodDepthM and HeightAboveFloodM are the values for
	// the triggered scenario.
	ScenarioStatus    string   `json:"scenario_status"`
	FloodDepthM       *float64 `json:"flood_depth_m"`
	HeightAboveFloodM *float64 `json:"height_above_flood_m"`
	Subscribers       int      `json:"subscribers"`
}

type Summary struct {
	Recipients int `json:"recipients"`
	Sent       int `json:"sent"`
	Failed     int `json:"failed"`
}
