import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
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
 * REQ-011 reset-password.
 *
 * /reset-password/:token. On success the backend establishes a session
 * cookie, so we just hydrate the session store and head to /dashboard.
 */
const FormSchema = z
  .object({
    password: z.string().min(12, 'Pick at least 12 characters.').max(200),
    confirm: z.string().min(1, 'Confirm your password'),
  })
  .refine((data) => data.password === data.confirm, {
    path: ['confirm'],
    message: 'Passwords do not match.',
  });
type FormValues = z.infer<typeof FormSchema>;

export default function ResetPassword(): JSX.Element {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const setSession = useSession((s) => s.setSession);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(FormSchema) });

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null);
    if (!token) {
      setSubmitError('This reset link is missing its token.');
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
      const msg =
        err instanceof ApiError
          ? err.message
          : 'Something went wrong. Please request a new reset link.';
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
          <h1 className="text-xl font-semibold text-gray-900">Pick a new password</h1>
          <p className="mt-1 text-sm text-gray-600">
            12 characters or more. We'll sign you in once it's set.
          </p>
          <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
            <Input
              label="New password"
              type="password"
              autoComplete="new-password"
              error={errors.password?.message}
              {...register('password')}
            />
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
        </div>
      </div>
    </div>
  );
}
