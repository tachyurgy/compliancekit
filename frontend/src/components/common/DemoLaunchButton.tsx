import { useState } from 'react';
import { useNavigate } from 'react-router-dom';

import Button from '@/components/common/Button';
import { apiFetch } from '@/api/client';
import { providersApi } from '@/api/providers';
import { useSession } from '@/hooks/useSession';

/**
 * One-click entry into a private, pre-seeded sandbox tenant (public demo
 * only). POST /api/auth/demo provisions the tenant and sets the session
 * cookie; we then load /api/me exactly like a normal sign-in.
 */
export default function DemoLaunchButton({
  size = 'lg',
  fullWidth = false,
  label = 'Explore the live demo',
}: {
  size?: 'sm' | 'md' | 'lg';
  fullWidth?: boolean;
  label?: string;
}): JSX.Element {
  const navigate = useNavigate();
  const setSession = useSession((s) => s.setSession);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const start = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      await apiFetch('/api/auth/demo', { method: 'POST', skipAuthRedirect: true });
      const me = await providersApi.me();
      if (me) setSession(me);
      navigate('/dashboard', { replace: true });
    } catch {
      setError('Could not start the demo. Please try again in a moment.');
      setBusy(false);
    }
  };

  return (
    <div className={fullWidth ? 'w-full' : undefined}>
      <Button size={size} fullWidth={fullWidth} loading={busy} onClick={() => void start()}>
        {busy ? 'Setting up your sandbox…' : label}
      </Button>
      {error ? (
        <p className="mt-2 text-sm text-critical-700" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
