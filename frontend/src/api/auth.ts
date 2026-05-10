/**
 * REQ-002 + REQ-011 auth API client.
 *
 * Sign-up/sign-in are still owned by `providersApi` (in providers.ts) for
 * back-compat with the current /signup and /login pages. This module adds
 * the new endpoints: forgot-password, reset-password, sign-out-everywhere,
 * list-sessions.
 */
import { z } from 'zod';
import { apiFetch } from './client';

export const SessionSchema = z.object({
  id: z.string(),
  ip: z.string(),
  user_agent: z.string(),
  last_seen_at: z.string(),
  expires_at: z.string(),
  created_at: z.string(),
  is_current: z.boolean(),
});
export type AuthSession = z.infer<typeof SessionSchema>;

export const authApi = {
  /** REQ-011: { email } → 200, always (no leak of which emails exist). */
  async forgotPassword(email: string): Promise<void> {
    await apiFetch('/api/auth/forgot-password', {
      method: 'POST',
      json: { email },
      skipAuthRedirect: true,
    });
  },

  /** REQ-011: { token, password, remember_me? } → auto-signin on success. */
  async resetPassword(input: {
    token: string;
    password: string;
    rememberMe?: boolean;
  }): Promise<{ user_id: string; provider_id: string }> {
    return apiFetch<{ user_id: string; provider_id: string }>(
      '/api/auth/reset-password',
      {
        method: 'POST',
        json: {
          token: input.token,
          password: input.password,
          remember_me: input.rememberMe ?? true,
        },
        skipAuthRedirect: true,
      },
    );
  },

  /** REQ-011: list active sessions for /settings/security. */
  async listSessions(): Promise<AuthSession[]> {
    const data = await apiFetch<{ sessions: unknown }>('/api/auth/sessions');
    const arr = Array.isArray(data?.sessions) ? data.sessions : [];
    return arr.map((s) => SessionSchema.parse(s));
  },

  async signoutEverywhere(): Promise<void> {
    await apiFetch('/api/auth/signout-everywhere', { method: 'POST' });
  },

  async signout(): Promise<void> {
    await apiFetch('/api/auth/signout', {
      method: 'POST',
      skipAuthRedirect: true,
    });
  },
};
