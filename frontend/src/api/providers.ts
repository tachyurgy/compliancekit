import { z } from 'zod';
import { apiFetch } from './client';

/**
 * The "provider" is the daycare organization. One owner per provider for MVP.
 *
 * NOTE on tolerant fields: the launch markets are FL/CA/TX, but pilot accounts
 * and older test data can carry stateCode '' or any 2-letter state. The Settings
 * page failed entirely with "Couldn't load your facility" when Zod refused
 * those rows because the enum was strict. We accept any string here and gate
 * the marketing-state logic at the UI layer instead. Same for licenseType —
 * legacy rows can be empty strings.
 */
export const ProviderSchema = z.object({
  id: z.string(),
  name: z.string(),
  stateCode: z.string(),
  licenseType: z.string(),
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

  /**
   * REQ-002 signup. The backend accepts both the legacy field names
   * (name/owner_email/state_code) AND the new ones (facility_name/email/
   * full_name) — we send both so we don't break existing call paths.
   */
  async signup(input: {
    name: string;
    ownerEmail: string;
    stateCode: 'CA' | 'TX' | 'FL';
    password: string;
    fullName?: string;
  }): Promise<{ providerId: string; userId: string }> {
    const data = await apiFetch<{ provider_id: string; user_id: string }>(
      '/api/auth/signup',
      {
        method: 'POST',
        json: {
          name: input.name,
          facility_name: input.name,
          owner_email: input.ownerEmail,
          email: input.ownerEmail,
          state_code: input.stateCode,
          password: input.password,
          full_name: input.fullName ?? input.name,
        },
        skipAuthRedirect: true,
      },
    );
    return { providerId: data.provider_id, userId: data.user_id };
  },

  /**
   * REQ-011 signin. `rememberMe` defaults to true (90-day sliding session);
   * pass false from /login when the operator unchecks "Remember me".
   */
  async signin(
    email: string,
    password: string,
    rememberMe = true,
  ): Promise<{ providerId: string; userId: string }> {
    const data = await apiFetch<{ provider_id: string; user_id: string }>(
      '/api/auth/signin',
      {
        method: 'POST',
        json: { email, password, remember_me: rememberMe },
        skipAuthRedirect: true,
      },
    );
    return { providerId: data.provider_id, userId: data.user_id };
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
