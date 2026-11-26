package zpa

import (
	"net/http"
	"strings"

	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/rs/zerolog/log"
)

// The ZPA answers a failed request with a web page as often as with a message. A
// planner reading the GUI can do nothing with markup, so only a short plain-text
// answer is passed on; everything else goes to the log.
const maxDetail = 300

// detail returns the ZPA's own words if they are short plain text, else "".
func detail(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" || len(s) > maxDetail || strings.Contains(s, "<") {
		return ""
	}
	return s
}

// unreachable reports that no answer came back at all.
func unreachable(what string, err error) error {
	log.Error().Err(err).Str("what", what).Msg("cannot reach ZPA")
	return apierr.Newf(apierr.CodeExternalZPA,
		"Das ZPA war nicht erreichbar (%s). Das liegt am Netz oder am ZPA selbst, nicht an den Daten.", what)
}

// refused reports a non-2xx answer. what names the request in the planner's words.
func refused(what string, resp *http.Response, body []byte) error {
	log.Error().Str("what", what).Str("status", resp.Status).Str("body", bodySnippet(body)).
		Msg("ZPA refused a request")
	var msg string
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg = "Das ZPA hat den Zugriff abgelehnt (" + resp.Status + ", " + what + "). " +
			"Die ZPA-Zugangsdaten des Plexams-Servers sind ungültig, oder der Server darf von seiner Adresse aus nicht auf das ZPA zugreifen. " +
			"Das muss die Plexams-Betreuung klären."
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		msg = "Das ZPA hat die Daten nicht angenommen (" + resp.Status + ", " + what + ")."
	default:
		msg = "Das ZPA hat mit einem Fehler geantwortet (" + resp.Status + ", " + what + ")."
	}
	if d := detail(body); d != "" {
		msg += " Meldung des ZPA: " + d
	}
	return apierr.New(apierr.CodeExternalZPA, msg)
}

// garbled reports an answer that is not the expected JSON.
func garbled(what string, body []byte) error {
	log.Error().Str("what", what).Str("body", bodySnippet(body)).Msg("ZPA sent an unexpected response")
	return apierr.Newf(apierr.CodeExternalZPA,
		"Das ZPA hat etwas Unerwartetes geantwortet (%s). Vermutlich ist dort gerade eine Wartung oder die Anmeldung des Plexams-Servers abgelaufen.", what)
}
