// Shared types for the in-house PDF e-signature component.
//
// Coordinate system:
//   PDF pages use a bottom-left origin with y increasing upward and units in
//   "PDF points" (1/72 inch). The browser renders with top-left origin, so
//   conversion is handled at the component boundary (FieldOverlay + pdfStamp).
//   All normalized coordinates stored on `Field` are in PDF-point space with
//   origin bottom-left; this matches pdf-lib's drawImage call signature.

export type FieldType = "signature" | "initial" | "date" | "text" | "checkbox";

export interface PageCoord {
  pageIndex: number; // 0-based
  x: number; // PDF points from left edge of page
  y: number; // PDF points from bottom edge of page
  width: number; // PDF points
  height: number; // PDF points
}

export interface Field extends PageCoord {
  id: string;
  type: FieldType;
  required: boolean;
  label?: string;
  assignedSignerId?: string; // for multi-party signing later
  defaultValue?: string;
}

export interface FieldValue {
  fieldId: string;
  type: FieldType;
  // Exactly one of the following is set depending on type.
  signaturePngBase64?: string;
  textValue?: string;
  checkboxChecked?: boolean;
}

export interface SignSession {
  token: string;
  documentId: string;
  documentUrl: string;
  signerName: string;
  signerEmail: string;
  providerId: string;
  fields: Field[];
  esignDisclosureVersion: string;
  expiresAt: string; // ISO-8601
  status: "pending" | "in_progress" | "completed" | "expired" | "revoked";
}

export interface ConsentRecord {
  esignDisclosureVersion: string;
  acceptedAt: string; // ISO-8601
  signerTypedName: string;
}

export interface AuditRecord {
  schemaVersion: "1.0";
  signatureId: string; // assigned by client; server may overwrite
  sessionToken: string;
  documentId: string;
  providerId: string;
  signerName: string;
  signerEmail: string;
  signedAt: string; // ISO-8601 UTC
  ipAddress?: string; // populated server-side; client leaves blank
  userAgent: string;
  sha256Before: string; // hash of original PDF bytes
  sha256After: string; // hash of stamped PDF bytes (pre-audit-page insertion? see spec)
  consent: ConsentRecord;
  fieldValues: Array<{
    fieldId: string;
    type: FieldType;
    pageIndex: number;
    // never include raw signature PNG in audit record; reference by field id
    filled: boolean;
  }>;
  clientTimeZone?: string;
  clientClockSkewMs?: number;
}

export interface SignatureRecord {
  signatureId: string;
  signedAt: string;
  sha256After: string;
  signedPdfS3Key: string;
  auditTrailS3Key: string;
}

export interface PreparedSession extends SignSession {
  // echo of session; kept separate for future auth-token wrapping
}
