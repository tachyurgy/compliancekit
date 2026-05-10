/**
 * REQ-016 client: attendance + warnings + drill roster.
 *
 * The kiosk surface uses the unauthenticated kiosk-bearer endpoints
 * (signIn / signOut / kioskRoster). The operator dashboard reads the
 * authenticated /api/attendance/today shape that includes warnings and
 * the 9:30 AM absence list.
 */
import { z } from 'zod';
import { apiFetch } from './client';

export const WarningSchema = z.object({
  code: z.string(),
  headline: z.string(),
  action: z.string(),
});
export type AttendanceWarning = z.infer<typeof WarningSchema>;

export const ChildPresentSchema = z.object({
  id: z.string(),
  first_name: z.string(),
  last_name: z.string(),
  age_months: z.number().int(),
  parent_name: z.string().optional().default(''),
  parent_phone: z.string().optional().default(''),
  allergies: z.string().optional().default(''),
  signed_in_at: z.string(),
});
export type ChildPresent = z.infer<typeof ChildPresentSchema>;

export const StaffPresentSchema = z.object({
  id: z.string(),
  first_name: z.string(),
  last_name: z.string(),
  role: z.string(),
  signed_in_at: z.string(),
});
export type StaffPresent = z.infer<typeof StaffPresentSchema>;

export const BandStatsSchema = z.object({
  label: z.string(),
  kids: z.number().int(),
  per_staff: z.number().int(),
  required: z.number().int(),
});

export const ByBandSchema = z.object({
  under_1: BandStatsSchema,
  m12_24: BandStatsSchema,
  m24_36: BandStatsSchema,
  m36_48: BandStatsSchema,
  m48_60: BandStatsSchema,
  m60_plus: BandStatsSchema,
});
export type ByBand = z.infer<typeof ByBandSchema>;

export const AbsenceSchema = z.object({
  child_id: z.string(),
  first_name: z.string(),
  last_name: z.string(),
  parent_name: z.string().optional().default(''),
  parent_phone: z.string().optional().default(''),
});
export type Absence = z.infer<typeof AbsenceSchema>;

export const TodaySchema = z.object({
  children_present: z.array(ChildPresentSchema),
  staff_present: z.array(StaffPresentSchema),
  by_band: ByBandSchema,
  required_floor: z.number().int(),
  warnings: z.array(WarningSchema),
  absences: z.array(AbsenceSchema),
  as_of: z.string(),
});
export type AttendanceToday = z.infer<typeof TodaySchema>;

export const attendanceApi = {
  async today(): Promise<AttendanceToday> {
    const data = await apiFetch<unknown>('/api/attendance/today');
    return TodaySchema.parse(data);
  },

  async issueKioskSession(label?: string): Promise<{ kiosk_session: string; expires_at: string }> {
    return apiFetch('/api/kiosk/session', { method: 'POST', json: { label: label ?? '' } });
  },

  async bulkRotatePins(scope: 'child' | 'staff', ids?: string[]): Promise<{ pins: Record<string, string>; count: number }> {
    return apiFetch('/api/settings/kiosk/rotate-pins', {
      method: 'POST',
      json: { scope, ids: ids ?? [] },
    });
  },

  /** URL for the inline-rendered drill roster PDF. */
  drillRosterURL(): string {
    return `${import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'}/api/drill-roster.pdf`;
  },
};

// ---- kiosk-bearer endpoints (no operator session required) ----
//
// The mounted iPad holds the kiosk session ID in localStorage. We attach it
// to every event request as `?ks=...` so a stale browser session can't be
// hijacked into making operator-side mutations.

const KIOSK_BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export interface KioskSignInInput {
  kind: 'child_in' | 'child_out' | 'staff_in' | 'staff_out';
  subject_id: string;
  pin: string;
  dropped_off_by?: string;
  picked_up_by?: string;
  notes?: string;
}

export const kioskBearerApi = {
  async signIn(sessionId: string, input: KioskSignInInput): Promise<{ ok: boolean; message: string; recorded_at: string }> {
    const res = await fetch(`${KIOSK_BASE}/kiosk/sign-in?ks=${sessionId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Kiosk-Session': sessionId },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      const txt = await res.text();
      throw new Error(txt || `Sign-in failed (${res.status})`);
    }
    return res.json();
  },

  async signOut(sessionId: string, input: KioskSignInInput): Promise<{ ok: boolean; message: string; recorded_at: string }> {
    const res = await fetch(`${KIOSK_BASE}/kiosk/sign-out?ks=${sessionId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Kiosk-Session': sessionId },
      body: JSON.stringify(input),
    });
    if (!res.ok) throw new Error(await res.text());
    return res.json();
  },

  async roster(sessionId: string): Promise<{ children_present: number; staff_present: number }> {
    const res = await fetch(`${KIOSK_BASE}/kiosk/roster?ks=${sessionId}`, {
      headers: { 'X-Kiosk-Session': sessionId },
    });
    if (!res.ok) throw new Error(await res.text());
    return res.json();
  },
};
