package notify

import (
	"strings"
	"testing"
)

func TestRenderStaffPortalInvite_HappyPath(t *testing.T) {
	t.Parallel()
	subject, html, text, err := RenderStaffPortalInvite(StaffPortalInviteData{
		FirstName:    "Maria Gonzalez",
		FacilityName: "Sunny Days Learning Center",
		OperatorName: "Linda Chen",
		UploadURL:    "https://app.levelbrook.com/u/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(subject, "Sunny Days Learning Center") {
		t.Fatalf("subject missing facility name: %q", subject)
	}
	for _, want := range []string{
		"Maria", // first name only
		"Sunny Days Learning Center",
		"Linda Chen", // operator
		"https://app.levelbrook.com/u/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
		"7", // default ExpiresInDays
		"levelbrookteam@gmail.com",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if strings.Contains(html, "Maria Gonzalez") {
		t.Errorf("HTML rendered full name; expected first-name-only")
	}
}

func TestRenderStaffPortalInvite_HTMLEscapes(t *testing.T) {
	t.Parallel()
	_, html, _, err := RenderStaffPortalInvite(StaffPortalInviteData{
		FirstName:    "<script>alert(1)</script>",
		FacilityName: "<b>Naughty</b>",
		OperatorName: "<i>Op</i>",
		UploadURL:    "https://app.levelbrook.com/u/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, bad := range []string{"<script>", "<b>Naughty</b>", "<i>Op</i>"} {
		if strings.Contains(html, bad) {
			t.Fatalf("unescaped %q in HTML output", bad)
		}
	}
}

func TestRenderStaffPortalInvite_Defaults(t *testing.T) {
	t.Parallel()
	_, html, text, err := RenderStaffPortalInvite(StaffPortalInviteData{
		UploadURL: "https://app.levelbrook.com/u/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"there", "Your director", "7"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing default %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text missing default %q", want)
		}
	}
}

func TestRenderParentPortalInvite_HappyPath(t *testing.T) {
	t.Parallel()
	subject, html, text, err := RenderParentPortalInvite(ParentPortalInviteData{
		FirstName:      "Jamal Carter",
		ChildFirstName: "Maya Carter",
		FacilityName:   "Sunny Days Learning Center",
		OperatorName:   "Linda Chen",
		UploadURL:      "https://app.levelbrook.com/u/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(subject, "Maya") || !strings.Contains(subject, "Sunny Days Learning Center") {
		t.Fatalf("subject missing tokens: %q", subject)
	}
	for _, want := range []string{
		"Jamal", // first name only
		"Maya",  // child first name only
		"Sunny Days Learning Center",
		"Linda Chen",
		"https://app.levelbrook.com/u/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
		"7",
		"levelbrookteam@gmail.com",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if strings.Contains(html, "Jamal Carter") {
		t.Errorf("HTML rendered parent's full name; expected first-name-only")
	}
	if strings.Contains(html, "Maya Carter") {
		t.Errorf("HTML rendered child's full name; expected first-name-only")
	}
}

func TestRenderParentPortalInvite_HTMLEscapes(t *testing.T) {
	t.Parallel()
	_, html, _, err := RenderParentPortalInvite(ParentPortalInviteData{
		FirstName:      "<script>p</script>",
		ChildFirstName: "<script>c</script>",
		FacilityName:   "<b>f</b>",
		OperatorName:   "<i>o</i>",
		UploadURL:      "https://app.levelbrook.com/u/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, bad := range []string{"<script>", "<b>f</b>", "<i>o</i>"} {
		if strings.Contains(html, bad) {
			t.Fatalf("unescaped %q in HTML output", bad)
		}
	}
}

func TestRenderParentPortalInvite_Defaults(t *testing.T) {
	t.Parallel()
	_, html, text, err := RenderParentPortalInvite(ParentPortalInviteData{
		UploadURL: "https://app.levelbrook.com/u/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// HTML auto-escapes apostrophes (' → &#39;), so assert the escaped
	// version for fragments that contain one. Plain-text body keeps the
	// literal apostrophe.
	htmlWants := []string{"there", "your child", "Your child&#39;s center", "7"}
	textWants := []string{"there", "your child", "Your child's center", "7"}
	for _, want := range htmlWants {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing default %q", want)
		}
	}
	for _, want := range textWants {
		if !strings.Contains(text, want) {
			t.Errorf("text missing default %q", want)
		}
	}
}
