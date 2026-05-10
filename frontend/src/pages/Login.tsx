import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { ShieldCheck } from 'lucide-react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import { emailSchema } from '@/lib/validation';
import { zodResolver } from '@/lib/zodResolver';
import { providersApi } from '@/api/providers';
import { ApiError } from '@/api/client';
import { useSession } from '@/hooks/useSession';

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
    defaultValues: { rememberMe: true },
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
            ? 'Email or password is incorrect.'
            : err.message
          : 'Something went wrong. Please try again.';
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
          <h1 className="text-xl font-semibold text-gray-900">Sign in</h1>
          <p className="mt-1 text-sm text-gray-600">Welcome back.</p>

          <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
            <Input
              label="Email"
              type="email"
              autoComplete="email"
              error={errors.email?.message}
              {...register('email')}
            />
            <Input
              label="Password"
              type="password"
              autoComplete="current-password"
              error={errors.password?.message}
              {...register('password')}
            />

            <div className="flex items-center justify-between">
              <label className="inline-flex items-center gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                  {...register('rememberMe')}
                />
                Remember me on this device
              </label>
              <Link
                to="/forgot-password"
                className="text-sm text-brand-600 hover:underline"
              >
                Forgot password?
              </Link>
            </div>

            {submitError ? (
              <p className="text-sm text-red-600" role="alert">
                {submitError}
              </p>
            ) : null}

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-gray-600">
            Don't have an account?{' '}
            <Link to="/signup" className="text-brand-600 hover:underline">
              Create one
            </Link>
          </p>
        </div>
      </div>
    </div>
  );
}
