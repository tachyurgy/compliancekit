package notify

// REQ-010: render the YC-tier staff-portal-invite email — sent when a
// manager invites a staff member to the magic-link upload portal
// (REQ-005). Mirrors the welcome.go pattern.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/staff-portal-invite.html
var staffPortalInviteHTML string

//go:embed templates/staff-portal-invite.txt
var staffPortalInviteTXT string

// StaffPortalInviteExpiryDays is the TTL the body advertises. Should match
// magiclink.TTLFor(KindStaffUpload) (currently 7 days). Surfaced so the
// caller can override if the link uses a non-default TTL.
const StaffPortalInviteExpiryDays = 7

// StaffPortalInviteData fills the staff-portal-invite template.
//
// FirstName       — recipient (staff) first name; falls back to "there".
// FacilityName    — daycare display name.
// OperatorName    — sender's name (the manager/director); the staff member
//
//	replies to this person via Reply-To. Falls back to "Your director".
//
// UploadURL       — absolute /u/<token> URL.
// ExpiresInDays   — TTL in days surfaced in body; defaults to
//
//	StaffPortalInviteExpiryDays when zero.
type StaffPortalInviteData struct {
	FirstName     string
	FacilityName  string
	OperatorName  string
	UploadURL     string
	ExpiresInDays int
}

// RenderStaffPortalInvite returns (subject, htmlBody, plainBody).
func RenderStaffPortalInvite(d StaffPortalInviteData) (subject, html, text string, err error) {
	if d.ExpiresInDays == 0 {
		d.ExpiresInDays = StaffPortalInviteExpiryDays
	}
	if d.FirstName == "" {
		d.FirstName = "there"
	} else {
		d.FirstName = firstNameOnly(d.FirstName)
	}
	if d.OperatorName == "" {
		d.OperatorName = "Your director"
	}
	if d.FacilityName == "" {
		d.FacilityName = "your child care center"
	}

	subject = fmt.Sprintf("Upload your documents for %s", d.FacilityName)

	htmlT, err := template.New("staff-portal-invite.html").Parse(staffPortalInviteHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse staff-portal-invite.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render staff-portal-invite.html: %w", err)
	}

	txtT, err := texttemplate.New("staff-portal-invite.txt").Parse(staffPortalInviteTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse staff-portal-invite.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render staff-portal-invite.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}
