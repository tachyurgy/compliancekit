import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import AuthShell from '@/components/common/AuthShell';
import { emailSchema } from '@/lib/validation';
import { zodResolver } from '@/lib/zodResolver';
import { providersApi } from '@/api/providers';
import { ApiError } from '@/api/client';
import { useSession } from '@/hooks/useSession';
import DemoLaunchButton from '@/components/common/DemoLaunchButton';
import { DEMO_MODE } from '@/lib/demo';
import { DEMO_EMAIL, DEMO_PASSWORD } from '@/components/common/DemoDeploymentNotice';

/**
 * REQ-011 sign-in page. Adds:
 *   - "Remember me on this device" (checked by default → 90-day sliding session).
 *   - "Forgot password?" link to /forgot-password.
 */
const FormSchema = z.object({
  email: emailSchema,
  password: z.string().min(1, 'Password is required'),
  rememberMe: z.boolean(),
});
type FormValues = z.infer<typeof FormSchema>;

export default function Login(): JSX.Element {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const setSession = useSession((s) => s.setSession);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(FormSchema),
    // Demo build: pre-fill the shared demo login so one click signs in.
    defaultValues: DEMO_MODE
      ? { email: DEMO_EMAIL, password: DEMO_PASSWORD, rememberMe: true }
      : { rememberMe: true },
  });

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null);
    try {
      await providersApi.signin(values.email, values.password, values.rememberMe);
      const me = await providersApi.me();
      if (me) setSession(me);
      const next = searchParams.get('next');
      const dest =
        me && !me.onboardingComplete
          ? '/onboarding'
          : next && next.startsWith('/')
            ? next
            : '/dashboard';
      navigate(dest, { replace: true });
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.status === 401
            ? "That email or password didn't match. Try again, or use Forgot password."
            : err.message
          : "Something went wrong on our end. Please try again in a minute.";
      setSubmitError(msg);
    }
  });

  return (
    <AuthShell
      title="Welcome back"
      description="Sign in to keep your center inspection-ready."
      footer={
        <>
          New to Levelbrook?{' '}
          <Link to="/signup" className="font-medium text-brand-700 hover:underline">
            Create your account
          </Link>
        </>
      }
    >
      {DEMO_MODE ? (
        <div className="mb-6 rounded-lb-sm bg-brand-50 px-4 py-4 ring-1 ring-brand-100">
          <p className="text-sm text-ink-800">
            This is a public demo with fictional data. Jump into a private
            sandbox of your own, or sign in below with the shared demo login.
          </p>
          <div className="mt-3">
            <DemoLaunchButton size="md" fullWidth label="Open a private demo sandbox" />
          </div>
        </div>
      ) : null}
      <form onSubmit={onSubmit} className="space-y-5" noValidate>
        <Input
          label="Email"
          type="email"
          autoComplete="email"
          error={errors.email?.message}
          {...register('email')}
        />
        {DEMO_MODE ? (
          <div
            data-testid="demo-credentials"
            className="rounded-lb-sm border border-brand-200 bg-brand-50/60 px-4 py-3"
          >
            <p className="text-xs font-semibold uppercase tracking-wide text-brand-700">
              Demo login
            </p>
            <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-ink-700">Email</dt>
              <dd className="font-mono text-ink-900 select-all">{DEMO_EMAIL}</dd>
              <dt className="text-ink-700">Password</dt>
              <dd className="font-mono text-ink-900 select-all">{DEMO_PASSWORD}</dd>
            </dl>
          </div>
        ) : null}
        <Input
          label="Password"
          type="password"
          autoComplete="current-password"
          error={errors.password?.message}
          {...register('password')}
        />

        <div className="flex items-center justify-between flex-wrap gap-3">
          <label className="inline-flex items-center gap-2 text-sm text-ink-800">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-line text-brand-600 focus:ring-brand-500"
              {...register('rememberMe')}
            />
            Remember me on this device
          </label>
          <Link
            to="/forgot-password"
            className="text-sm font-medium text-brand-700 hover:underline"
          >
            Forgot password?
          </Link>
        </div>

        {submitError ? (
          <p
            className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
            role="alert"
          >
            {submitError}
          </p>
        ) : null}

        <Button type="submit" fullWidth loading={isSubmitting}>
          {isSubmitting ? 'Signing in…' : 'Sign in'}
        </Button>
      </form>
    </AuthShell>
  );
}
