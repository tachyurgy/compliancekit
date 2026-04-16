package models

import (
	"time"
)

type StateCode string

const (
	StateCA StateCode = "CA"
	StateTX StateCode = "TX"
	StateFL StateCode = "FL"
	StateOR StateCode = "OR"
	StateWA StateCode = "WA"
)

type Provider struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	LegalName     string    `json:"legal_name,omitempty"`
	StateCode     StateCode `json:"state_code"`
	LicenseNumber string    `json:"license_number,omitempty"`
	OwnerEmail    string    `json:"owner_email"`
	OwnerPhone    string    `json:"owner_phone,omitempty"`
	Capacity      int       `json:"capacity"`
	Timezone      string    `json:"timezone"`
	StripeCustID  string    `json:"stripe_customer_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
}

type Child struct {
	ID           string    `json:"id"`
	ProviderID   string    `json:"provider_id"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	DOB          time.Time `json:"date_of_birth"`
	EnrollDate   time.Time `json:"enroll_date"`
	ParentEmail  string    `json:"parent_email,omitempty"`
	ParentPhone  string    `json:"parent_phone,omitempty"`
	Classroom    string    `json:"classroom,omitempty"`
	Status       string    `json:"status"` // enrolled | withdrawn | pending
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Staff struct {
	ID              string    `json:"id"`
	ProviderID      string    `json:"provider_id"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Role            string    `json:"role"` // director | teacher | aide | cook
	Email           string    `json:"email"`
	Phone           string    `json:"phone,omitempty"`
	HireDate        time.Time `json:"hire_date"`
	BackgroundCheck *time.Time `json:"background_check_date,omitempty"`
	Status          string    `json:"status"` // active | terminated
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DocumentKind drives the chase/compliance logic.
type DocumentKind string

const (
	DocImmunization    DocumentKind = "immunization_record"
	DocEmergencyCard   DocumentKind = "emergency_card"
	DocEnrollmentForm  DocumentKind = "enrollment_form"
	DocPhysicalExam    DocumentKind = "physical_exam"
	DocTBTest          DocumentKind = "tb_test"
	DocCPRCert         DocumentKind = "cpr_cert"
	DocFirstAidCert    DocumentKind = "first_aid_cert"
	DocBackgroundCheck DocumentKind = "background_check"
	DocLicense         DocumentKind = "facility_license"
	DocInsurance       DocumentKind = "liability_insurance"
	DocFireInspection  DocumentKind = "fire_inspection"
	DocFoodHandler     DocumentKind = "food_handler_cert"
	DocOther           DocumentKind = "other"
)

type Document struct {
	ID               string       `json:"id"`
	ProviderID       string       `json:"provider_id"`
	SubjectKind      string       `json:"subject_kind"` // child | staff | facility
	SubjectID        string       `json:"subject_id,omitempty"`
	Kind             DocumentKind `json:"kind"`
	Title            string       `json:"title"`
	StorageBucket    string       `json:"storage_bucket"`
	StorageKey       string       `json:"storage_key"`
	MIMEType         string       `json:"mime_type"`
	SizeBytes        int64        `json:"size_bytes"`
	IssuedAt         *time.Time   `json:"issued_at,omitempty"`
	ExpiresAt        *time.Time   `json:"expires_at,omitempty"`
	OCRConfidence    float64      `json:"ocr_confidence,omitempty"`
	OCRSource        string       `json:"ocr_source,omitempty"` // mistral | gemini | manual
	UploadedBy       string       `json:"uploaded_by,omitempty"`
	UploadedVia      string       `json:"uploaded_via,omitempty"` // provider | parent_portal | staff_portal
	LastChaseSentAt  *time.Time   `json:"last_chase_sent_at,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	DeletedAt        *time.Time   `json:"deleted_at,omitempty"`
}

type Subscription struct {
	ID                 string     `json:"id"`
	ProviderID         string     `json:"provider_id"`
	StripeSubID        string     `json:"stripe_subscription_id"`
	StripePriceID      string     `json:"stripe_price_id"`
	Status             string     `json:"status"` // trialing | active | past_due | canceled
	CurrentPeriodEnd   time.Time  `json:"current_period_end"`
	CancelAt           *time.Time `json:"cancel_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type MagicKind string

const (
	MagicProviderSignup MagicKind = "provider_signup"
	MagicProviderSignin MagicKind = "provider_signin"
	MagicParentUpload   MagicKind = "parent_upload"
	MagicStaffUpload    MagicKind = "staff_upload"
	MagicDocumentSign   MagicKind = "document_sign"
)

type MagicLinkToken struct {
	ID         string    `json:"id"`
	Kind       MagicKind `json:"kind"`
	SubjectID  string    `json:"subject_id"` // provider.id | child.id | staff.id | document.id
	ProviderID string    `json:"provider_id"`
	TokenHash  []byte    `json:"-"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type AuditLogEntry struct {
	ID         string                 `json:"id"`
	ProviderID string                 `json:"provider_id"`
	Actor      string                 `json:"actor"`  // user id, email, or "system"
	Action     string                 `json:"action"` // e.g. "document.upload"
	TargetKind string                 `json:"target_kind"`
	TargetID   string                 `json:"target_id"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type PolicyVersion struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	Kind       string    `json:"kind"` // parent_handbook | staff_handbook | emergency_plan
	Version    int       `json:"version"`
	StorageKey string    `json:"storage_key"`
	EffectiveAt time.Time `json:"effective_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type Signature struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	DocumentID string    `json:"document_id"`
	SignerKind string    `json:"signer_kind"` // parent | staff | provider
	SignerID   string    `json:"signer_id"`
	SignerName string    `json:"signer_name"`
	SignedAt   time.Time `json:"signed_at"`
	IPAddress  string    `json:"ip_address,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	PDFKey     string    `json:"pdf_storage_key,omitempty"` // set by pdfsign package
}
