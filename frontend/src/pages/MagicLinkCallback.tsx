import { useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams, Link } from 'react-router-dom';
import { ShieldCheck, AlertTriangle } from 'lucide-react';

import LoadingSpinner from '@/components/common/LoadingSpinner';
import Button from '@/components/common/Button';
import { providersApi } from '@/api/providers';
import { useSession } from '@/hooks/useSession';
import { ApiError } from '@/api/client';
import { isLikelyBase62Token } from '@/lib/base62';

export default function MagicLinkCallback(): JSX.Element {
  const { token } = useParams<{ token: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const setSession = useSession((s) => s.setSession);

  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!token || !isLikelyBase62Token(token)) {
        setError('This sign-in link is malformed. Please request a new one.');
        return;
      }
      try {
        const user = await providersApi.consumeMagicLink(token);
        if (cancelled) return;
        setSession(user);
        const next = searchParams.get('next');
        const dest = user.onboardingComplete
          ? next && next.startsWith('/')
            ? next
            : '/dashboard'
          : '/onboarding';
        navigate(dest, { replace: true });
      } catch (err) {
        if (cancelled) return;
        const msg =
          err instanceof ApiError
            ? err.status === 410 || err.status === 404
              ? 'This sign-in link has expired or already been used.'
              : err.message
            : 'Something went wrong. Please try again.';
        setError(msg);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token, searchParams, navigate, setSession]);

  return (
    <div className="min-h-screen grid place-items-center bg-surface-muted p-6">
      <div className="w-full max-w-md card p-6 text-center">
        <Link to="/" className="inline-flex items-center gap-2 mb-6">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold">ComplianceKit</span>
        </Link>
        {error ? (
          <>
            <div className="mx-auto mb-4 h-12 w-12 rounded-full bg-critical-50 grid place-items-center">
              <AlertTriangle className="h-6 w-6 text-critical-600" />
            </div>
            <h1 className="text-lg font-semibold text-gray-900">Sign-in failed</h1>
            <p className="mt-2 text-sm text-gray-600">{error}</p>
            <div className="mt-5">
              <Link to="/login">
                <Button>Request a new link</Button>
              </Link>
            </div>
          </>
        ) : (
          <div className="flex flex-col items-center py-6">
            <LoadingSpinner label="Signing you in..." size="lg" />
          </div>
        )}
      </div>
    </div>
  );
}
