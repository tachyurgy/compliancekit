import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { Mail } from 'lucide-react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import AuthShell from '@/components/common/AuthShell';
import { emailSchema } from '@/lib/validation';
import { zodResolver } from '@/lib/zodResolver';
import { authApi } from '@/api/auth';

/**
 * REQ-011 forgot-password.
 *
 * The backend always returns 200 — we don't reveal whether the email exists.
 * The UI mirrors that with a single confirmation message regardless of input.
 */
const FormSchema = z.object({
  email: emailSchema,
});
type FormValues = z.infer<typeof FormSchema>;

export default function ForgotPassword(): JSX.Element {
  const [sent, setSent] = useState(false);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(FormSchema) });

  const onSubmit = handleSubmit(async (values) => {
    await authApi.forgotPassword(values.email);
    setSent(true);
  });

  if (sent) {
    return (
      <AuthShell title="Check your email">
        <div className="text-center">
          <div
            className="mx-auto h-14 w-14 rounded-full bg-brand-50 ring-1 ring-brand-100 grid place-items-center"
            aria-hidden
          >
            <Mail className="h-6 w-6 text-brand-600" />
          </div>
          <p className="mt-5 text-sm text-ink-700 leading-relaxed">
            If there's a Levelbrook account for that email, you'll get a
            reset link in a minute. The link works for 30 minutes — check
            spam if you don't see it.
          </p>
          <Link
            to="/login"
            className="mt-7 inline-block text-sm font-medium text-brand-700 hover:underline"
          >
            Back to sign in
          </Link>
        </div>
      </AuthShell>
    );
  }

  return (
    <AuthShell
      title="Reset your password"
      description={
        <>Type the email you signed up with. We'll send you a link to pick a new password.</>
      }
      footer={
        <>
          Remembered it?{' '}
          <Link to="/login" className="font-medium text-brand-700 hover:underline">
            Sign in
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-5" noValidate>
        <Input
          label="Email address"
          type="email"
          autoComplete="email"
          error={errors.email?.message}
          {...register('email')}
        />
        <Button type="submit" fullWidth loading={isSubmitting}>
          {isSubmitting ? 'Sending…' : 'Email me a reset link'}
        </Button>
      </form>
    </AuthShell>
  );
}
