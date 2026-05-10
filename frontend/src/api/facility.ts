import { z } from 'zod';
import { apiFetch } from './client';

// Facility row — REQ-003 onboarding wizard state. The backend treats the
// providers row as the facility table; this schema is the camelCase shape
// the frontend talks to (see backend/internal/handlers/facility.go).
export const FacilitySchema = z.object({
  id: z.string(),
  name: z.string().default(''),
  stateCode: z.string().default(''),
  licenseNumber: z.string().default(''),
  licenseExpiresOn: z.string().nullable().optional(),
  county: z.string().default(''),
  address1: z.string().default(''),
  address2: z.string().default(''),
  city: z.string().default(''),
  stateRegion: z.string().default(''),
  postalCode: z.string().default(''),
  phone: z.string().default(''),
  capacity: z.number().int().default(0),
  indoorSqft: z.number().int().nullable().optional(),
  outdoorPlayground: z.boolean().default(false),
  eveningLicense: z.boolean().default(false),
  weekendLicense: z.boolean().default(false),
  twentyFourHour: z.boolean().default(false),
  ownerEmail: z.string().default(''),
  ownerFullName: z.string().default(''),
  directorEmail: z.string().default(''),
  directorFullName: z.string().default(''),
  onboardingCompletedAt: z.string().nullable().optional(),
});
export type Facility = z.infer<typeof FacilitySchema>;

export type FacilityPatch = Partial<Omit<Facility, 'id' | 'stateCode' | 'ownerEmail' | 'onboardingCompletedAt'>>;

// FL's age bands. These match Handbook §§ 3.4 ratio rules and are the
// only valid choices for room.ageBand on the FL launch.
export const FL_AGE_BANDS = [
  { value: 'infant_0_11', label: 'Infants (0–11 mo)' },
  { value: 'ones', label: '1-year-olds' },
  { value: 'twos', label: '2-year-olds' },
  { value: 'threes', label: '3-year-olds' },
  { value: 'fours', label: '4-year-olds' },
  { value: 'fives', label: '5-year-olds' },
  { value: 'school_age', label: 'School age' },
] as const;

export type FLAgeBand = (typeof FL_AGE_BANDS)[number]['value'];

export const RoomSchema = z.object({
  id: z.string(),
  name: z.string(),
  ageBand: z.string().default(''),
  licensedCapacity: z.number().int().nullable().optional(),
  indoorSqft: z.number().int().nullable().optional(),
  createdAt: z.string(),
});
export type Room = z.infer<typeof RoomSchema>;

export interface RoomInput {
  name: string;
  ageBand?: string;
  licensedCapacity?: number;
  indoorSqft?: number;
}

export const facilityApi = {
  async get(): Promise<Facility> {
    const data = await apiFetch<unknown>('/api/facility');
    return FacilitySchema.parse(data);
  },
  async patch(input: FacilityPatch): Promise<Facility> {
    const data = await apiFetch<unknown>('/api/facility', {
      method: 'PATCH',
      json: input,
    });
    return FacilitySchema.parse(data);
  },
  async completeOnboarding(): Promise<Facility> {
    const data = await apiFetch<unknown>('/api/facility/complete-onboarding', {
      method: 'POST',
    });
    return FacilitySchema.parse(data);
  },
  async listRooms(): Promise<Room[]> {
    const data = await apiFetch<unknown>('/api/rooms');
    return z.array(RoomSchema).parse(data);
  },
  async createRoom(input: RoomInput): Promise<{ id: string }> {
    return apiFetch<{ id: string }>('/api/rooms', { method: 'POST', json: input });
  },
  async patchRoom(id: string, input: RoomInput): Promise<void> {
    await apiFetch(`/api/rooms/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      json: input,
    });
  },
  async deleteRoom(id: string): Promise<void> {
    await apiFetch(`/api/rooms/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },
};
