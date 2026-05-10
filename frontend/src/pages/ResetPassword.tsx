import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';
import { ShieldCheck } from 'lucide-react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import { zodResolver } from '@/lib/zodResolver';
import { authApi } from '@/api/auth';
import { ApiError } from '@/api/client';
import { providersApi } from '@/api/providers';
import { useSession } from '@/hooks/useSession';

/**
 * REQ-011 / REQ-034 reset-password.
 *
 * /reset-password/:token. On success the backend establishes a session
 * cookie, so we just hydrate the session store and head to /dashboard.
 *
 * Brand-voice copy lives in REQ-034:
 *   - "Pick at least 12 characters." (verbatim, matches the backend message).
 *   - "These don't match — please retype." for confirm-mismatch.
 *   - 401 from the API → the link is invalid or expired; show the dedicated
 *     "This link doesn't work anymore" panel with a Send a new link CTA
 *     pointing back at /forgot-password.
 */
const FormSchema = z
  .object({
    password: z.string().min(12, 'Pick at least 12 characters.').max(200),
    confirm: z.string().min(1, 'Confirm your password.'),
  })
  .refine((data) => data.password === data.confirm, {
    path: ['confirm'],
    message: "These don't match — please retype.",
  });
type FormValues = z.infer<typeof FormSchema>;

export default function ResetPassword(): JSX.Element {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const setSession = useSession((s) => s.setSession);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [linkDead, setLinkDead] = useState(false);

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(FormSchema), mode: 'onChange' });

  // Live strength hint as the user types.
  const passwordValue = useWatch({ control, name: 'password' }) ?? '';
  const passwordLen = passwordValue.length;

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null);
    setLinkDead(false);
    if (!token) {
      setLinkDead(true);
      return;
    }
    try {
      await authApi.resetPassword({
        token,
        password: values.password,
        rememberMe: true,
      });
      const me = await providersApi.me();
      if (me) setSession(me);
      navigate(me && !me.onboardingComplete ? '/onboarding' : '/dashboard', {
        replace: true,
      });
    } catch (err) {
      // 401 from /api/auth/reset-password means the token is bad / expired /
      // already used. Swap the form for the dedicated "link doesn't work"
      // panel so the user has a single clear next action.
      if (err instanceof ApiError && err.status === 401) {
        setLinkDead(true);
        return;
      }
      const msg =
        err instanceof ApiError
          ? err.message
          : "Something went wrong on our end. Please try again in a minute.";
      setSubmitError(msg);
    }
  });

  return (
    <div className="min-h-screen grid place-items-center bg-surface-muted p-6">
      <div className="w-full max-w-md">
        <Link to="/" className="flex items-center gap-2 mb-6">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold text-lg">Levelbrook</span>
        </Link>
        <div className="card p-6">
          {linkDead ? (
            <div className="text-center">
              <h1 className="text-xl font-semibold text-gray-900">
                This link doesn't work anymore
              </h1>
              <p className="mt-2 text-sm text-gray-600">
                Reset links are good for 30 minutes — yours has expired or
                already been used. Request a fresh one and we'll email it
                right over.
              </p>
              <Link
                to="/forgot-password"
                className="mt-6 inline-flex w-full justify-center rounded-md bg-brand-600 px-4 py-2 text-sm font-semibold text-white hover:bg-brand-700"
              >
                Send a new link
              </Link>
              <Link
                to="/login"
                className="mt-4 inline-block text-sm text-gray-600 hover:underline"
              >
                Back to sign in
              </Link>
            </div>
          ) : (
            <>
              <h1 className="text-xl font-semibold text-gray-900">Pick a new password</h1>
              <p className="mt-1 text-sm text-gray-600">
                12 characters or more. We'll sign you in once it's set.
              </p>
              <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
                <div>
                  <Input
                    label="New password"
                    type="password"
                    autoComplete="new-password"
                    error={errors.password?.message}
                    {...register('password')}
                  />
                  {/* Live strength hint — only shown once the user starts typing. */}
                  {passwordLen > 0 && !errors.password ? (
                    <p
                      className={`mt-1 text-xs ${
                        passwordLen >= 12 ? 'text-green-700' : 'text-gray-500'
                      }`}
                    >
                      {passwordLen >= 12
                        ? "Looks good."
                        : `${passwordLen}/12 characters`}
                    </p>
                  ) : null}
                </div>
                <Input
                  label="Confirm new password"
                  type="password"
                  autoComplete="new-password"
                  error={errors.confirm?.message}
                  {...register('confirm')}
                />
                {submitError ? (
                  <p className="text-sm text-red-600" role="alert">
                    {submitError}
                  </p>
                ) : null}
                <Button type="submit" className="w-full" disabled={isSubmitting}>
                  {isSubmitting ? 'Saving…' : 'Save and sign in'}
                </Button>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
