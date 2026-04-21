import { z } from 'zod';
import { apiFetch } from './client';

/** The "provider" is the daycare organization. One owner per provider for MVP. */
export const ProviderSchema = z.object({
  id: z.string(),
  name: z.string(),
  stateCode: z.enum(['CA', 'TX', 'FL']),
  licenseType: z.enum(['center', 'family_home']),
  licenseNumber: z.string().nullable(),
  address1: z.string(),
  address2: z.string().nullable(),
  city: z.string(),
  stateRegion: z.string(),
  postalCode: z.string(),
  capacity: z.number().int(),
  agesServedMonths: z.object({
    minMonths: z.number().int(),
    maxMonths: z.number().int(),
  }),
  onboardingComplete: z.boolean(),
  createdAt: z.string(),
});
export type Provider = z.infer<typeof ProviderSchema>;

export const SessionUserSchema = z.object({
  id: z.string(),
  email: z.string().email(),
  fullName: z.string().nullable(),
  providerId: z.string(),
  role: z.enum(['owner', 'director', 'staff']),
  onboardingComplete: z.boolean(),
});
export type SessionUser = z.infer<typeof SessionUserSchema>;

export const providersApi = {
  async me(): Promise<SessionUser | null> {
    try {
      const data = await apiFetch<unknown>('/api/me', { skipAuthRedirect: true });
      return SessionUserSchema.parse(data);
    } catch {
      return null;
    }
  },

  async getProvider(): Promise<Provider> {
    const data = await apiFetch<unknown>('/api/provider');
    return ProviderSchema.parse(data);
  },

  async updateProvider(input: Partial<Omit<Provider, 'id' | 'createdAt'>>): Promise<Provider> {
    const data = await apiFetch<unknown>('/api/provider', {
      method: 'PATCH',
      json: input,
    });
    return ProviderSchema.parse(data);
  },

  async requestMagicLink(email: string): Promise<{ ok: true }> {
    await apiFetch('/api/auth/magic-link', {
      method: 'POST',
      json: { email },
      skipAuthRedirect: true,
    });
    return { ok: true };
  },

  async consumeMagicLink(token: string): Promise<SessionUser> {
    const data = await apiFetch<unknown>(`/api/auth/callback/${encodeURIComponent(token)}`, {
      method: 'POST',
      skipAuthRedirect: true,
    });
    return SessionUserSchema.parse(data);
  },

  async logout(): Promise<void> {
    await apiFetch('/api/auth/logout', { method: 'POST', skipAuthRedirect: true });
  },

  async completeOnboarding(payload: {
    stateCode: 'CA' | 'TX' | 'FL';
    licenseType: 'center' | 'family_home';
    licenseNumber?: string;
    name: string;
    address1: string;
    address2?: string;
    city: string;
    stateRegion: string;
    postalCode: string;
    capacity: number;
    agesServedMonths: { minMonths: number; maxMonths: number };
    staff?: Array<{
      firstName: string;
      lastName: string;
      email?: string;
      role: 'director' | 'lead_teacher' | 'assistant' | 'aide' | 'cook' | 'other';
    }>;
    children?: Array<{
      firstName: string;
      lastName: string;
      dateOfBirth: string;
      parentEmail?: string;
    }>;
  }): Promise<Provider> {
    const data = await apiFetch<unknown>('/api/provider/onboarding', {
      method: 'POST',
      json: payload,
    });
    return ProviderSchema.parse(data);
  },
};
