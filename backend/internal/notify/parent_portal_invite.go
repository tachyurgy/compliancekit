package notify

// REQ-010: render the YC-tier parent-portal-invite email — sent when a
// manager invites a parent to upload child documents via magic link
// (REQ-005).

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/parent-portal-invite.html
var parentPortalInviteHTML string

//go:embed templates/parent-portal-invite.txt
var parentPortalInviteTXT string

// ParentPortalInviteExpiryDays is the TTL the body advertises. Should match
// magiclink.TTLFor(KindParentUpload) (currently 7 days).
const ParentPortalInviteExpiryDays = 7

// ParentPortalInviteData fills the parent-portal-invite template.
//
// FirstName        — parent first name; falls back to "there".
// ChildFirstName   — the child whose docs need uploading; falls back to "your child".
// FacilityName     — daycare display name.
// OperatorName     — sender (manager/director); falls back to "Your child's center".
// UploadURL        — absolute /u/<token> URL.
// ExpiresInDays    — TTL in days surfaced in body; defaults to
//                    ParentPortalInviteExpiryDays when zero.
type ParentPortalInviteData struct {
	FirstName      string
	ChildFirstName string
	FacilityName   string
	OperatorName   string
	UploadURL      string
	ExpiresInDays  int
}

// RenderParentPortalInvite returns (subject, htmlBody, plainBody).
func RenderParentPortalInvite(d ParentPortalInviteData) (subject, html, text string, err error) {
	if d.ExpiresInDays == 0 {
		d.ExpiresInDays = ParentPortalInviteExpiryDays
	}
	if d.FirstName == "" {
		d.FirstName = "there"
	} else {
		d.FirstName = firstNameOnly(d.FirstName)
	}
	if d.ChildFirstName == "" {
		d.ChildFirstName = "your child"
	} else {
		d.ChildFirstName = firstNameOnly(d.ChildFirstName)
	}
	if d.OperatorName == "" {
		d.OperatorName = "Your child's center"
	}
	if d.FacilityName == "" {
		d.FacilityName = "your child's center"
	}

	subject = fmt.Sprintf("Upload %s's forms for %s", d.ChildFirstName, d.FacilityName)

	htmlT, err := template.New("parent-portal-invite.html").Parse(parentPortalInviteHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse parent-portal-invite.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render parent-portal-invite.html: %w", err)
	}

	txtT, err := texttemplate.New("parent-portal-invite.txt").Parse(parentPortalInviteTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse parent-portal-invite.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render parent-portal-invite.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}
