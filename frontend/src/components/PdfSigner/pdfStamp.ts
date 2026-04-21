import { PDFDocument, StandardFonts, rgb } from "pdf-lib";
import type { AuditRecord } from "./types";

export async function sha256(bytes: Uint8Array): Promise<string> {
  // Copy into a fresh ArrayBuffer-backed Uint8Array so subtle.digest accepts
  // the BufferSource shape cleanly under TS 5.6+'s stricter typing.
  const copy = new Uint8Array(bytes);
  const digest = await crypto.subtle.digest("SHA-256", copy);
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

export interface StampSignatureOpts {
  pageIndex: number;
  /** PDF-point coordinates, origin bottom-left. */
  x: number;
  y: number;
  width: number;
  height: number;
  pngBytes: Uint8Array;
}

export async function stampSignature(
  pdfBytes: Uint8Array,
  opts: StampSignatureOpts,
): Promise<Uint8Array> {
  const pdf = await PDFDocument.load(pdfBytes);
  const page = pdf.getPage(opts.pageIndex);
  const png = await pdf.embedPng(opts.pngBytes);
  page.drawImage(png, {
    x: opts.x,
    y: opts.y,
    width: opts.width,
    height: opts.height,
  });
  return pdf.save({ useObjectStreams: false });
}

export interface StampTextOpts {
  pageIndex: number;
  x: number;
  y: number;
  fontSize?: number;
  text: string;
}

export async function stampText(
  pdfBytes: Uint8Array,
  opts: StampTextOpts,
): Promise<Uint8Array> {
  const pdf = await PDFDocument.load(pdfBytes);
  const font = await pdf.embedFont(StandardFonts.Helvetica);
  const page = pdf.getPage(opts.pageIndex);
  page.drawText(opts.text, {
    x: opts.x,
    y: opts.y,
    size: opts.fontSize ?? 11,
    font,
    color: rgb(0, 0, 0),
  });
  return pdf.save({ useObjectStreams: false });
}

export interface StampCheckboxOpts {
  pageIndex: number;
  x: number;
  y: number;
  size: number;
  checked: boolean;
}

export async function stampCheckbox(
  pdfBytes: Uint8Array,
  opts: StampCheckboxOpts,
): Promise<Uint8Array> {
  const pdf = await PDFDocument.load(pdfBytes);
  const page = pdf.getPage(opts.pageIndex);
  const { x, y, size, checked } = opts;
  page.drawRectangle({
    x,
    y,
    width: size,
    height: size,
    borderColor: rgb(0, 0, 0),
    borderWidth: 1,
  });
  if (checked) {
    // Draw an X via two overlapping lines.
    page.drawLine({
      start: { x, y },
      end: { x: x + size, y: y + size },
      thickness: 1.2,
      color: rgb(0, 0, 0),
    });
    page.drawLine({
      start: { x, y: y + size },
      end: { x: x + size, y },
      thickness: 1.2,
      color: rgb(0, 0, 0),
    });
  }
  return pdf.save({ useObjectStreams: false });
}

/**
 * Appends a standardized audit/certificate page to the end of the PDF.
 * Rendered inline from the audit record so the certificate is visible even if
 * the external JSON is lost. The JSON remains canonical; this page is just a
 * human-readable echo.
 */
export async function appendAuditPage(
  pdfBytes: Uint8Array,
  audit: AuditRecord,
): Promise<Uint8Array> {
  const pdf = await PDFDocument.load(pdfBytes);
  const font = await pdf.embedFont(StandardFonts.Helvetica);
  const bold = await pdf.embedFont(StandardFonts.HelveticaBold);

  const page = pdf.addPage([612, 792]); // US Letter
  const { height } = page.getSize();
  const left = 54;
  let cursorY = height - 72;

  page.drawText("Electronic Signature Certificate", {
    x: left,
    y: cursorY,
    size: 18,
    font: bold,
    color: rgb(0, 0, 0),
  });
  cursorY -= 28;

  page.drawText(
    "This page certifies the electronic signature applied to this document under the",
    { x: left, y: cursorY, size: 10, font, color: rgb(0.2, 0.2, 0.2) },
  );
  cursorY -= 12;
  page.drawText(
    "U.S. ESIGN Act (15 U.S.C. \u00a77001 et seq.) and applicable state UETA provisions.",
    { x: left, y: cursorY, size: 10, font, color: rgb(0.2, 0.2, 0.2) },
  );
  cursorY -= 24;

  const rows: Array<[string, string]> = [
    ["Signature ID", audit.signatureId],
    ["Document ID", audit.documentId],
    ["Signed By", `${audit.signerName} <${audit.signerEmail}>`],
    ["Signed At (UTC)", audit.signedAt],
    ["Typed Name (Consent)", audit.consent.signerTypedName],
    ["ESIGN Disclosure", audit.consent.esignDisclosureVersion],
    ["Consent Accepted", audit.consent.acceptedAt],
    ["User Agent", truncate(audit.userAgent, 80)],
    ["SHA-256 (original)", audit.sha256Before],
    ["SHA-256 (signed)", audit.sha256After],
  ];
  if (audit.ipAddress) rows.push(["Signer IP", audit.ipAddress]);
  if (audit.clientTimeZone) rows.push(["Signer Time Zone", audit.clientTimeZone]);

  for (const [label, value] of rows) {
    page.drawText(label, { x: left, y: cursorY, size: 10, font: bold });
    page.drawText(value, { x: left + 140, y: cursorY, size: 10, font });
    cursorY -= 16;
    if (cursorY < 80) break;
  }

  cursorY -= 10;
  page.drawText(
    "Tampering will invalidate the SHA-256 chain recorded in the ComplianceKit audit trail.",
    { x: left, y: cursorY, size: 9, font, color: rgb(0.35, 0.35, 0.35) },
  );

  return pdf.save({ useObjectStreams: false });
}

function truncate(s: string, n: number): string {
  return s.length <= n ? s : s.slice(0, n - 1) + "\u2026";
}

/** Convert a data URL (e.g. from signature_pad.toDataURL) to raw PNG bytes. */
export function dataUrlToBytes(dataUrl: string): Uint8Array {
  const comma = dataUrl.indexOf(",");
  if (comma < 0) throw new Error("invalid data url");
  const b64 = dataUrl.slice(comma + 1);
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}
