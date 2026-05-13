import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
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

/**
 * REQ-002 signup page. Posts to /api/auth/signup → /onboarding/checkout.
 * State defaults to FL — Florida is the launch market and operators can
 * change it in the onboarding wizard if they're TX/CA.
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
      navigate('/onboarding/checkout', { replace: true });
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Something went wrong on our end. Please try again in a minute.";
      setSubmitError(msg);
    }
  });

  return (
    <AuthShell
      title="Start your free trial"
      description="14 days free, no charge today. You can cancel any time."
      footer={
        <>
          Already with us?{' '}
          <Link to="/login" className="font-medium text-brand-700 hover:underline">
            Sign in
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-5" noValidate>
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
          <p
            className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
            role="alert"
          >
            {submitError}
          </p>
        ) : null}

        <Button type="submit" fullWidth loading={isSubmitting}>
          {isSubmitting ? 'Creating your account…' : 'Create account and continue'}
        </Button>
        <p className="text-xs text-ink-500 text-center leading-relaxed">
          We add your card on the next step. You won't be charged until day 15.
        </p>
      </form>
    </AuthShell>
  );
}
