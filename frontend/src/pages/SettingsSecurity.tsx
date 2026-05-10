import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ShieldAlert, Laptop } from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { authApi } from '@/api/auth';
import { toast } from '@/components/common/Toast';
import { ApiError } from '@/api/client';

/**
 * REQ-011: list active sessions; "Sign out everywhere" revokes all of them
 * including the current one. Browser is then bounced to /login on the next
 * authenticated fetch.
 */
export default function SettingsSecurity(): JSX.Element {
  const qc = useQueryClient();
  const { data: sessions, isLoading } = useQuery({
    queryKey: ['auth', 'sessions'],
    queryFn: () => authApi.listSessions(),
  });

  const signoutAll = useMutation({
    mutationFn: () => authApi.signoutEverywhere(),
    onSuccess: () => {
      toast.success('Signed out of every device.');
      qc.invalidateQueries({ queryKey: ['auth', 'sessions'] });
      // Slight delay so the toast renders before the redirect.
      setTimeout(() => {
        window.location.assign('/login');
      }, 600);
    },
    onError: (err) =>
      toast.error(err instanceof ApiError ? err.message : "Couldn't sign out."),
  });

  return (
    <div>
      <Link
        to="/settings"
        className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 mb-4"
      >
        <ArrowLeft className="h-4 w-4" /> Back to settings
      </Link>
      <PageHeader
        title="Security"
        description="Devices that are signed in to your Levelbrook account."
      />

      {isLoading ? (
        <LoadingSpinner />
      ) : (
        <div className="space-y-3">
          {(sessions ?? []).map((s) => (
            <Card key={s.id} padded>
              <div className="flex items-start gap-3">
                <Laptop className="h-5 w-5 text-gray-400 mt-1" />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-medium text-gray-900">
                      {summarizeUserAgent(s.user_agent)}
                    </span>
                    {s.is_current && (
                      <span className="text-xs font-medium px-2 py-0.5 rounded-full bg-brand-100 text-brand-700">
                        This device
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-xs text-gray-500">
                    IP {s.ip || '—'} · last active{' '}
                    {new Date(s.last_seen_at).toLocaleString()}
                  </p>
                </div>
              </div>
            </Card>
          ))}
          {sessions && sessions.length === 0 && (
            <Card padded>
              <p className="text-sm text-gray-600">No active sessions.</p>
            </Card>
          )}

          <Card padded>
            <div className="flex items-start gap-3">
              <ShieldAlert className="h-5 w-5 text-critical-600 mt-0.5" />
              <div className="flex-1">
                <div className="font-medium text-gray-900">
                  Sign out everywhere
                </div>
                <p className="mt-1 text-sm text-gray-600">
                  Revokes every session, including this one. You'll need to
                  sign in again.
                </p>
                <Button
                  variant="danger"
                  size="sm"
                  className="mt-3"
                  loading={signoutAll.isPending}
                  onClick={() => signoutAll.mutate()}
                >
                  Sign out everywhere
                </Button>
              </div>
            </div>
          </Card>
        </div>
      )}
    </div>
  );
}

/**
 * Trim a user-agent string down to "Chrome on macOS" or similar. Best-effort —
 * we don't ship a full UA parser; the goal is just a friendlier label than
 * the raw string.
 */
function summarizeUserAgent(ua: string): string {
  if (!ua) return 'Unknown device';
  let browser = 'Browser';
  if (ua.includes('Firefox/')) browser = 'Firefox';
  else if (ua.includes('Edg/')) browser = 'Edge';
  else if (ua.includes('Chrome/')) browser = 'Chrome';
  else if (ua.includes('Safari/')) browser = 'Safari';

  let os = 'Unknown OS';
  if (ua.includes('Mac OS X') || ua.includes('Macintosh')) os = 'macOS';
  else if (ua.includes('Windows')) os = 'Windows';
  else if (ua.includes('Android')) os = 'Android';
  else if (ua.includes('iPhone') || ua.includes('iPad')) os = 'iOS';
  else if (ua.includes('Linux')) os = 'Linux';
  return `${browser} on ${os}`;
}
