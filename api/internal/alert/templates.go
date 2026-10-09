package alert

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"text/template"
)

// Warning texts, one per language. Edit freely. The fields available are
// those of messageData; every number comes straight from the data files.
var messageTemplates = map[string]*template.Template{
	LangHindi: template.Must(template.New("hi").Parse(
		`चेतावनी: {{.LakeName}} से बाढ़ का खतरा। ` +
			`आपके गाँव{{if .PlaceName}} {{.PlaceName}}{{end}} तक पानी ` +
			`{{if .ArrivalMinFast}}लगभग {{.ArrivalMinFast}} मिनट में{{else}}जल्द{{end}} पहुँच सकता है। ` +
			`तुरंत ऊँचे स्थान पर जाएँ।`)),
	LangEnglish: template.Must(template.New("en").Parse(
		`WARNING: Flood danger from {{.LakeName}}. ` +
			`Water may reach your village{{if .PlaceName}} {{.PlaceName}}{{end}} ` +
			`{{if .ArrivalMinFast}}in about {{.ArrivalMinFast}} minutes{{else}}soon{{end}}. ` +
			`Move to high ground immediately.`)),
}

// titles head push notifications and voice calls.
var titles = map[string]string{
	LangHindi:   "आहट: बाढ़ चेतावनी",
	LangEnglish: "Aahat: flood warning",
}

func title(lang string) string {
	if t, ok := titles[lang]; ok {
		return t
	}
	return titles[LangHindi]
}

// smsTemplates are short versions for SMS: a Hindi (Unicode) SMS part holds only 70 characters.
var smsTemplates = map[string]*template.Template{
	LangHindi: template.Must(template.New("hi-sms").Parse(
		`बाढ़ चेतावनी: {{.PlaceName}} में ~{{.ArrivalMinFast}} मिनट में पानी। ऊँचे स्थान पर जाएँ`)),
	LangEnglish: template.Must(template.New("en-sms").Parse(
		`FLOOD WARNING: water may reach {{.PlaceName}} in ~{{.ArrivalMinFast}} min. Go to high ground now.`)),
}

func renderShort(lang string, d messageData) string {
	t, ok := smsTemplates[lang]
	if !ok {
		t = smsTemplates[LangHindi]
	}
	var b bytes.Buffer
	if t.Execute(&b, d) != nil {
		return ""
	}
	return b.String()
}

// messageData is what a template sees. Names are already in the message's
// language where the data has them.
type messageData struct {
	LakeName       string // with "झील"/"lake" appended, see lakeLabel
	PlaceName      string
	ArrivalMinFast string // empty when the data has no arrival time
}

// lakeLabel appends the word for "lake" unless the name already ends with it
// (the index has both "घेपन घाट झील" and "लम डल").
func lakeLabel(lang, name string) string {
	if lang == LangHindi {
		if strings.HasSuffix(name, "झील") {
			return name
		}
		return name + " झील"
	}
	if strings.HasSuffix(strings.ToLower(name), "lake") {
		return name
	}
	return name + " lake"
}

func renderMessage(lang string, d messageData) (string, error) {
	t, ok := messageTemplates[lang]
	if !ok {
		t = messageTemplates[LangHindi]
	}
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

// formatMinutes prints whole minutes, rounded down so a warning never promises more time than the
// data gives (60.8 -> 60).
func formatMinutes(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(math.Floor(*v)))
}
