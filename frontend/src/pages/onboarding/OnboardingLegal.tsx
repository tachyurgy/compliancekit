import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';

import Button from '@/components/common/Button';
import BrandMark from '@/components/common/BrandMark';
import Input from '@/components/common/Input';
import MarkdownBox from '@/components/common/MarkdownBox';
import { legalApi } from '@/api/legal';
import { ApiError } from '@/api/client';
// Markdown is bundled via Vite's ?raw import — this is the exact text we
// render inline. The backend keeps a synchronized copy under
// internal/handlers/legaldocs/ which is what the audit-trail row hashes.
import tosMarkdown from '@/legal/tos.md?raw';
import dpaMarkdown from '@/legal/dpa.md?raw';

import { useSession } from '@/hooks/useSession';

/**
 * REQ-002 #6 + #7. The operator types their full legal name, ticks two
 * checkboxes, and clicks "Sign and continue". We POST /api/legal/sign which
 * writes one row to legal_signatures per document_kind with IP, UA, typed
 * name, version, and the SHA-256 of the rendered Markdown the backend
 * served. Then we hand off to /onboarding/facility (REQ-003 territory).
 */
export default function OnboardingLegal(): JSX.Element {
  const navigate = useNavigate();
  const user = useSession((s) => s.user);
  const rehydrate = useSession((s) => s.rehydrate);
  const [typedName, setTypedName] = useState('');
  const [authorized, setAuthorized] = useState(false);
  const [agree, setAgree] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sign = useMutation({
    mutationFn: () =>
      legalApi.sign({ typedName: typedName.trim(), documentKinds: ['tos', 'dpa'] }),
    onSuccess: async () => {
      // Backend flips onboarding_complete in the same tx as the signatures
      // (see backend/internal/handlers/legal.go). Refresh the session so the
      // RequireOnboarded guard lets us into /dashboard immediately.
      try {
        await rehydrate();
      } catch {
        // Rehydrate failures are non-fatal — the route guard will redirect
        // and the dashboard load will re-fetch.
      }
      navigate('/dashboard', { replace: true });
    },
    onError: (err) => {
      setError(
        err instanceof ApiError
          ? err.message
          : 'Something went wrong saving your signature. Try again.',
      );
    },
  });

  const canSubmit = typedName.trim().length >= 2 && authorized && agree;

  return (
    <div className="min-h-screen bg-surface-muted">
      <header className="border-b border-line bg-white">
        <div className="max-w-5xl mx-auto px-6 h-16 flex items-center">
          <BrandMark to="/" size="md" />
        </div>
      </header>

      <main className="max-w-3xl mx-auto px-6 py-12">
        <div>
          <span className="eyebrow">Step 2 of 3 — paperwork</span>
          <h1 className="mt-4 font-display text-4xl text-ink-900 tracking-tight leading-tight">
            Two quick agreements before you upload anything.
          </h1>
          <p className="mt-3 text-base text-ink-700 leading-relaxed">
            These protect your records and ours. They're written in plain English.
          </p>
        </div>

        <section className="mt-8 bg-white rounded-lb border border-line shadow-card p-6">
          <h2 className="text-lg font-semibold text-ink-900">Terms of Service</h2>
          <p className="mt-1 text-xs text-ink-500">Scroll through. Plain English, ~3 minutes.</p>
          <div className="mt-3">
            <MarkdownBox source={tosMarkdown} ariaLabel="Terms of Service" />
          </div>
        </section>

        <section className="mt-4 bg-white rounded-lb border border-line shadow-card p-6">
          <h2 className="text-lg font-semibold text-ink-900">Data Processing Addendum</h2>
          <p className="mt-1 text-xs text-ink-500">Where your records are stored and who can touch them.</p>
          <div className="mt-3">
            <MarkdownBox source={dpaMarkdown} ariaLabel="Data Processing Addendum" />
          </div>
        </section>

        <section className="mt-6 bg-white rounded-lb border border-line shadow-card p-6">
          <h2 className="text-lg font-semibold text-ink-900">Sign here</h2>
          <p className="mt-1.5 text-sm text-ink-700 leading-relaxed">
            Type your full legal name. Tick both boxes. We'll record this with
            today's date, your IP, and your browser — that's how an electronic
            signature works.
          </p>
          <div className="mt-5 space-y-4">
            <Input
              label="Your full legal name"
              autoComplete="name"
              value={typedName}
              onChange={(e) => setTypedName(e.target.value)}
              placeholder={user?.fullName ?? 'Maria L. Rodriguez'}
            />
            <label className="flex items-start gap-3 text-sm text-ink-800">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-line text-brand-600 focus:ring-brand-500"
                checked={authorized}
                onChange={(e) => setAuthorized(e.target.checked)}
              />
              <span>I am authorized to sign on behalf of my facility.</span>
            </label>
            <label className="flex items-start gap-3 text-sm text-ink-800">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-line text-brand-600 focus:ring-brand-500"
                checked={agree}
                onChange={(e) => setAgree(e.target.checked)}
              />
              <span>
                I have read and agree to the Terms of Service and the Data
                Processing Addendum.
              </span>
            </label>

            {error && (
              <p
                className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
                role="alert"
              >
                {error}
              </p>
            )}

            <Button
              fullWidth
              size="lg"
              disabled={!canSubmit}
              loading={sign.isPending}
              onClick={() => sign.mutate()}
            >
              Sign and continue
            </Button>
            <p className="text-xs text-ink-500 text-center">
              By clicking, you agree both documents apply to your account.
            </p>
          </div>
        </section>
      </main>
    </div>
  );
}
