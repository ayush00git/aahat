package alert

import (
	"bytes"
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

// formatMinutes prints the data value as is (60.8 stays 60.8).
func formatMinutes(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}
