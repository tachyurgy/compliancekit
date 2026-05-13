import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import AuthShell from '@/components/common/AuthShell';
import { zodResolver } from '@/lib/zodResolver';
import { authApi } from '@/api/auth';
import { ApiError } from '@/api/client';
import { providersApi } from '@/api/providers';
import { useSession } from '@/hooks/useSession';

/**
 * REQ-011 / REQ-034 reset-password. /reset-password/:token. On success the
 * backend sets a session cookie; we hydrate the session store and head to
 * /dashboard. Token-related failures swap the form for the dedicated
 * "This link doesn't work anymore" panel.
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
      if (err instanceof ApiError) {
        const tokenIssue =
          err.status === 401 ||
          (err.status === 400 && /reset link/i.test(err.message));
        if (tokenIssue) {
          setLinkDead(true);
          return;
        }
        setSubmitError(err.message);
        return;
      }
      setSubmitError(
        "Something went wrong on our end. Please try again in a minute.",
      );
    }
  });

  if (linkDead) {
    return (
      <AuthShell title="This link doesn't work anymore">
        <p className="text-sm text-ink-700 leading-relaxed">
          Reset links are good for 30 minutes — yours has expired or already
          been used. Request a fresh one and we'll email it right over.
        </p>
        <div className="mt-7 space-y-3">
          <Link to="/forgot-password" className="block">
            <Button fullWidth>Send a new link</Button>
          </Link>
          <Link
            to="/login"
            className="block text-center text-sm font-medium text-ink-700 hover:underline"
          >
            Back to sign in
          </Link>
        </div>
      </AuthShell>
    );
  }

  return (
    <AuthShell
      title="Pick a new password"
      description="12 characters or more. We'll sign you in once it's set."
    >
      <form onSubmit={onSubmit} className="space-y-5" noValidate>
        <div>
          <Input
            label="New password"
            type="password"
            autoComplete="new-password"
            error={errors.password?.message}
            {...register('password')}
          />
          {passwordLen > 0 && !errors.password ? (
            <p
              className={`mt-1.5 text-xs font-medium ${
                passwordLen >= 12 ? 'text-brand-700' : 'text-ink-500'
              }`}
            >
              {passwordLen >= 12 ? 'Looks good.' : `${passwordLen}/12 characters`}
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
          <p
            className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
            role="alert"
          >
            {submitError}
          </p>
        ) : null}
        <Button type="submit" fullWidth loading={isSubmitting}>
          {isSubmitting ? 'Saving…' : 'Save and sign in'}
        </Button>
      </form>
    </AuthShell>
  );
}
