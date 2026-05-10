import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { ShieldCheck } from 'lucide-react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
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
  const [typedName, setTypedName] = useState('');
  const [authorized, setAuthorized] = useState(false);
  const [agree, setAgree] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sign = useMutation({
    mutationFn: () =>
      legalApi.sign({ typedName: typedName.trim(), documentKinds: ['tos', 'dpa'] }),
    onSuccess: () => {
      navigate('/onboarding/facility', { replace: true });
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
      <header className="px-6 py-5 border-b border-gray-100 bg-white">
        <Link to="/" className="flex items-center gap-2">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold text-lg">Levelbrook</span>
        </Link>
      </header>

      <main className="max-w-3xl mx-auto px-6 py-10">
        <div>
          <h1 className="text-3xl font-semibold text-gray-900">
            Two quick agreements before you upload anything.
          </h1>
          <p className="mt-2 text-base text-gray-600">
            These protect your records and ours. They're written in plain English.
          </p>
        </div>

        <section className="mt-8 card p-6">
          <details open>
            <summary className="cursor-pointer text-lg font-semibold text-gray-900">
              Terms of Service
            </summary>
            <article className="prose prose-sm mt-4 max-w-none text-gray-800 whitespace-pre-wrap">
              {tosMarkdown}
            </article>
          </details>
        </section>

        <section className="mt-4 card p-6">
          <details open>
            <summary className="cursor-pointer text-lg font-semibold text-gray-900">
              Data Processing Addendum
            </summary>
            <article className="prose prose-sm mt-4 max-w-none text-gray-800 whitespace-pre-wrap">
              {dpaMarkdown}
            </article>
          </details>
        </section>

        <section className="mt-6 card p-6">
          <h2 className="text-lg font-semibold text-gray-900">Sign here</h2>
          <p className="mt-1 text-sm text-gray-600">
            Type your full legal name. Tick both boxes. We'll record this with
            today's date, your IP, and your browser — that's how an electronic
            signature works.
          </p>
          <div className="mt-4 space-y-4">
            <Input
              label="Your full legal name"
              autoComplete="name"
              value={typedName}
              onChange={(e) => setTypedName(e.target.value)}
              placeholder={user?.fullName ?? 'Maria L. Rodriguez'}
            />
            <label className="flex items-start gap-3 text-sm text-gray-800">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                checked={authorized}
                onChange={(e) => setAuthorized(e.target.checked)}
              />
              <span>
                I am authorized to sign on behalf of my facility.
              </span>
            </label>
            <label className="flex items-start gap-3 text-sm text-gray-800">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                checked={agree}
                onChange={(e) => setAgree(e.target.checked)}
              />
              <span>
                I have read and agree to the Terms of Service and the Data
                Processing Addendum.
              </span>
            </label>

            {error && (
              <p className="text-sm text-red-600" role="alert">
                {error}
              </p>
            )}

            <Button
              className="w-full"
              size="lg"
              disabled={!canSubmit}
              loading={sign.isPending}
              onClick={() => sign.mutate()}
            >
              Sign and continue
            </Button>
            <p className="text-xs text-gray-500 text-center">
              By clicking, you agree both documents apply to your account.
            </p>
          </div>
        </section>
      </main>
    </div>
  );
}
