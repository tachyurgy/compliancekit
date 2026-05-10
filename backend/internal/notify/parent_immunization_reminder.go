package notify

// REQ-022: branded parent immunization-record reminder.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates/parent-immunization-reminder.html
var parentImmunizationHTML string

//go:embed templates/parent-immunization-reminder.txt
var parentImmunizationTXT string

// ParentImmunizationReminderData fills the parent-immunization-reminder
// template's placeholders.
type ParentImmunizationReminderData struct {
	ChildFirstName string
	ChildLastName  string
	FacilityName   string
	ExpiresOnLong  string
	UploadURL      string
}

// RenderParentImmunizationReminder renders the parent immunization reminder.
// The caller passes expiresOnShort for the subject line ("Mon, Jul 12").
func RenderParentImmunizationReminder(d ParentImmunizationReminderData, expiresOnShort string) (subject, html, text string, err error) {
	subject = fmt.Sprintf("%s's immunization record expires %s", d.ChildFirstName, expiresOnShort)

	htmlT, err := template.New("parent-immunization-reminder.html").Parse(parentImmunizationHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse parent-immunization-reminder.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render parent-immunization-reminder.html: %w", err)
	}

	txtT, err := texttemplate.New("parent-immunization-reminder.txt").Parse(parentImmunizationTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse parent-immunization-reminder.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render parent-immunization-reminder.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

// IsImmunizationDoc decides whether a parent-facing chase email should use
// the immunization-specific template. The chase query carries only the
// document title, so we match against the FL launch vocabulary:
// "immunization", "DH 680" / "DH-680", "shot record", "vaccin*".
//
// False negatives fall back to the generic RenderChaseEmail path; false
// positives are extremely unlikely for FL launch data (the title field is
// human-typed by the operator from a known dropdown).
func IsImmunizationDoc(title string) bool {
	t := strings.ToLower(title)
	keywords := []string{"immuniz", "dh 680", "dh-680", "dh680", "shot record", "vaccin"}
	for _, k := range keywords {
		if strings.Contains(t, k) {
			return true
		}
	}
	return false
}
