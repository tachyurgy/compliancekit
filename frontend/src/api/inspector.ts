/**
 * Inspector Mode API client (REQ-009).
 *
 * Distinct from `inspections.ts`, which serves the pre-visit Inspection
 * Readiness Simulator. This module talks to /api/inspection/* (singular) —
 * the live event log that backs the iPad surface a real DCF Licensing
 * Counselor sees.
 *
 * No Zod here on purpose: Inspector Mode reads must never crash the demo.
 * Backend is the source of truth; we trust the JSON shape.
 */

import { apiFetch, API_BASE_URL } from './client';

export interface FacilityCard {
  id: string;
  name: string;
  license_number?: string;
  license_expiry?: string;
  address?: string;
  capacity: number;
  director_name?: string;
  state_code: string;
}

export interface LastInspection {
  started_at: string;
  ended_at?: string;
  counselor_name?: string;
  type?: string;
  findings_count: number;
}

export type StatusPill = 'On file' | 'Expiring soon' | 'Expired' | 'Missing';

export interface PersonnelItem {
  label: string;
  status: StatusPill;
  document_id?: string;
  expires_at?: string | null;
  form_name?: string;
}

export interface StaffCard {
  id: string;
  name: string;
  role?: string;
  hire_date?: string;
  items: PersonnelItem[];
}

export interface ChildCard {
  id: string;
  name: string;
  date_of_birth?: string;
  enrollment_date?: string;
  allergies?: string;
  medical_notes?: string;
  items: PersonnelItem[];
}

export interface RecordRow {
  label: string;
  form_name?: string;
  status: StatusPill;
  expires_at?: string;
  document_id?: string;
}

export interface PostingRow {
  kind: string;
  label: string;
  required: boolean;
  room_id?: string;
  photo_path?: string;
  last_verified_at?: string | null;
}

export interface DrillEntry {
  id: string;
  drill_kind: string;
  drill_date: string;
  notes?: string;
}

export interface DrillsBlock {
  window_days: number;
  logged: DrillEntry[];
  counts_by_kind: Record<string, number>;
}

export interface InspectorSnapshot {
  facility: FacilityCard;
  last_inspection?: LastInspection;
  staff_sample: StaffCard[];
  child_sample: ChildCard[];
  facility_records: RecordRow[];
  postings: PostingRow[];
  drills: DrillsBlock;
  open_session_id?: string;
  generated_at: string;
}

export async function getSnapshot(): Promise<InspectorSnapshot> {
  return apiFetch<InspectorSnapshot>('/api/inspection/snapshot');
}

export interface InspectionSession {
  id: string;
  facility_id: string;
  started_at: string;
  ended_at?: string;
  counselor_name?: string;
  type: string;
  sample: { staff_ids?: string[]; child_ids?: string[] };
  findings: Finding[];
  plan_of_correction_pdf_path?: string;
}

export interface Finding {
  line_number: string;
  class: string;
  text: string;
  note?: string;
  added_at: string;
}

export async function startSession(input: {
  counselor_name?: string;
  type?: 'routine' | 'renewal' | 'complaint' | 'followup' | 'other';
}): Promise<InspectionSession> {
  return apiFetch<InspectionSession>('/api/inspection/sessions', {
    method: 'POST',
    json: input,
  });
}

export async function pullSample(
  sessionId: string,
  input: { kind: 'staff' | 'child'; count: number },
): Promise<{ kind: string; ids: string[] }> {
  return apiFetch(`/api/inspection/sessions/${sessionId}/sample`, {
    method: 'POST',
    json: input,
  });
}

export async function addFinding(
  sessionId: string,
  input: { line_number: string; class: string; text: string; note?: string },
): Promise<{ finding: Finding; findings: Finding[] }> {
  return apiFetch(`/api/inspection/sessions/${sessionId}/finding`, {
    method: 'POST',
    json: input,
  });
}

export async function endSession(
  sessionId: string,
): Promise<InspectionSession> {
  return apiFetch<InspectionSession>(
    `/api/inspection/sessions/${sessionId}/end`,
    { method: 'POST', json: {} },
  );
}

export function planOfCorrectionURL(sessionId: string): string {
  return `${API_BASE_URL}/api/inspection/sessions/${sessionId}/plan-of-correction.pdf`;
}
