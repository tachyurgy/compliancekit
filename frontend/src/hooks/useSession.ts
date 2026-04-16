import { create } from 'zustand';
import { providersApi, type SessionUser } from '@/api/providers';

export type SessionStatus = 'loading' | 'authenticated' | 'anonymous';

interface SessionUserClient {
  id: string;
  email: string;
  fullName: string | null;
  providerId: string;
  role: 'owner' | 'director' | 'staff';
  onboardingComplete: boolean;
}

interface SessionState {
  user: SessionUserClient | null;
  status: SessionStatus;
  /** Hit /api/me and populate the store. Safe to call on mount every render. */
  rehydrate: () => Promise<void>;
  /** Set session after magic-link callback succeeds. */
  setSession: (user: SessionUser) => void;
  /** Clear session locally and call backend logout. */
  signOut: () => Promise<void>;
  /** Mark onboarding complete in the local store after the wizard finishes. */
  markOnboardingComplete: () => void;
}

let rehydratePromise: Promise<void> | null = null;

export const useSession = create<SessionState>((set, get) => ({
  user: null,
  status: 'loading',

  rehydrate: async () => {
    // De-dupe concurrent callers (React StrictMode double-invokes effects in dev).
    if (rehydratePromise) return rehydratePromise;
    rehydratePromise = (async () => {
      try {
        const me = await providersApi.me();
        if (me) {
          set({ user: { ...me }, status: 'authenticated' });
        } else {
          set({ user: null, status: 'anonymous' });
        }
      } finally {
        rehydratePromise = null;
      }
    })();
    return rehydratePromise;
  },

  setSession: (user) => {
    set({ user: { ...user }, status: 'authenticated' });
  },

  signOut: async () => {
    try {
      await providersApi.logout();
    } finally {
      set({ user: null, status: 'anonymous' });
    }
  },

  markOnboardingComplete: () => {
    const u = get().user;
    if (u) set({ user: { ...u, onboardingComplete: true } });
  },
}));
