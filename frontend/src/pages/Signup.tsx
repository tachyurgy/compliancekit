import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
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
 * REQ-002 signup page.
 *
 * Fields: full name, work email, password (min 12), facility name. State is
 * defaulted to FL — Florida is the launch market and we don't ask the
 * operator to pick something they're going to pick anyway. They can change
 * it during the onboarding wizard if they're TX/CA.
 *
 * On submit → POST /api/auth/signup → redirect to /onboarding/checkout.
 */
const FormSchema = z.object({
  fullName: z.string().min(1, "What's your name?").max(120),
  facilityName: z.string().min(1, 'Center name is required').max(200),
  email: emailSchema,
  password: z
    .string()
    .min(12, 'Pick at least 12 characters.')
    .max(200, "That's a lot of characters."),
});
type FormValues = z.infer<typeof FormSchema>;

export default function Signup(): JSX.Element {
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
    try {
      await providersApi.signup({
        name: values.facilityName,
        ownerEmail: values.email,
        stateCode: 'FL',
        password: values.password,
        fullName: values.fullName,
      });
      const me = await providersApi.me();
      if (me) setSession(me);
      // REQ-002: after signup, head to /onboarding/checkout (annual default).
      navigate('/onboarding/checkout', { replace: true });
    } catch (err) {
      const msg =
        err instanceof ApiError ? err.message : 'Something went wrong. Please try again.';
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
          <h1 className="text-xl font-semibold text-gray-900">Create your account</h1>
          <p className="mt-1 text-sm text-gray-600">
            14 days free, no charge today. You can cancel any time.
          </p>

          <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
            <Input
              label="Your full name"
              autoComplete="name"
              error={errors.fullName?.message}
              {...register('fullName')}
            />
            <Input
              label="Center name"
              autoComplete="organization"
              error={errors.facilityName?.message}
              {...register('facilityName')}
            />
            <Input
              label="Work email"
              type="email"
              autoComplete="email"
              error={errors.email?.message}
              {...register('email')}
            />
            <Input
              label="Password"
              type="password"
              autoComplete="new-password"
              error={errors.password?.message}
              hint="At least 12 characters."
              {...register('password')}
            />

            {submitError ? (
              <p className="text-sm text-red-600" role="alert">
                {submitError}
              </p>
            ) : null}

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? 'Creating account…' : 'Continue'}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-gray-600">
            Already have an account?{' '}
            <Link to="/login" className="text-brand-600 hover:underline">
              Sign in
            </Link>
          </p>
        </div>
      </div>
    </div>
  );
}
