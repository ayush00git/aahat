package alert

import (
	"github.com/ayush00git/aahat/api/internal/data"
)

// AffectedPlaces picks the settlements, schools and health facilities hit
// under the scenario, nearest first by fast-flood arrival, one row per OSM id.
func AffectedPlaces(impacts []data.Impact, scenario string) []data.Impact {
	var out []data.Impact
	seen := map[string]bool{}
	for _, im := range impacts {
		if !data.IsPeoplePlace(im.Kind) || !im.AffectedUnder(scenario) || seen[im.OSM] {
			continue
		}
		seen[im.OSM] = true
		out = append(out, im)
	}
	data.SortByArrival(out)
	return out
}

// BuildPlan joins the affected places (already in arrival order) with the
// subscriptions for them. Recipients come out in the same order, so the
// nearest village is warned first. A phone subscribed twice to the same place
// on the same channel gets one message.
func BuildPlan(lake data.Lake, scenario string, affected []data.Impact, subs []Subscription) ([]Recipient, []AffectedPlace, error) {
	byPlace := map[string][]Subscription{}
	for _, s := range subs {
		byPlace[s.PlaceOSM] = append(byPlace[s.PlaceOSM], s)
	}

	recipients := []Recipient{}
	places := make([]AffectedPlace, 0, len(affected))
	for _, im := range affected {
		place := AffectedPlace{
			OSM: im.OSM, Name: im.Name, NameHi: im.NameHi,
			Kind: im.Kind, Subkind: im.Subkind, Lon: im.Lon, Lat: im.Lat, Km: im.Km,
			ArrivalMinFast: im.ArrivalMinFast, ArrivalMinExpected: im.ArrivalMinExpected,
			Status: im.Status,
		}
		if o := im.Outcome(scenario); o != nil {
			place.ScenarioStatus = o.Status
			place.FloodDepthM = o.FloodDepthM
			place.HeightAboveFloodM = o.HeightAboveFloodM
		}

		sent := map[string]bool{}
		for _, s := range byPlace[im.OSM] {
			key := s.Phone + "|" + s.Channel
			if sent[key] {
				continue
			}
			sent[key] = true

			text, err := renderMessage(s.Lang, messageData{
				LakeName:       lakeLabel(s.Lang, lakeName(lake, s.Lang)),
				PlaceName:      placeName(im, s),
				ArrivalMinFast: formatMinutes(im.ArrivalMinFast),
			})
			if err != nil {
				return nil, nil, err
			}
			name := s.PlaceName
			if name == "" {
				name = data.Str(im.Name)
			}
			recipients = append(recipients, Recipient{
				SubscriptionID: s.ID, Phone: s.Phone, Lang: s.Lang, Channel: s.Channel,
				PlaceOSM: im.OSM, PlaceName: name,
				ArrivalMinFast: im.ArrivalMinFast, ArrivalMinExpected: im.ArrivalMinExpected,
				Status:   im.Status,
				Message:  text,
				Delivery: Delivery{Status: DeliveryPending},
			})
		}
		place.Subscribers = len(sent)
		places = append(places, place)
	}
	return recipients, places, nil
}

func lakeName(l data.Lake, lang string) string {
	if lang == LangHindi && l.NameHi != "" {
		return l.NameHi
	}
	return l.Name
}

// placeName prefers the name from the data (in the message's language), then
// the name the subscriber typed.
func placeName(im data.Impact, s Subscription) string {
	if lang := s.Lang; lang == LangHindi && data.Str(im.NameHi) != "" {
		return *im.NameHi
	}
	if n := data.Str(im.Name); n != "" {
		return n
	}
	return s.PlaceName
}
