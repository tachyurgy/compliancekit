import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { ShieldCheck, Mail } from 'lucide-react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
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

  return (
    <div className="min-h-screen grid place-items-center bg-surface-muted p-6">
      <div className="w-full max-w-md">
        <Link to="/" className="flex items-center gap-2 mb-6">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold text-lg">Levelbrook</span>
        </Link>
        <div className="card p-6">
          {sent ? (
            <div className="text-center">
              <Mail className="mx-auto h-10 w-10 text-brand-600" />
              <h1 className="mt-3 text-xl font-semibold text-gray-900">Check your email</h1>
              <p className="mt-2 text-sm text-gray-600">
                If that email is on file, we just sent a reset link. The link
                works for 30 minutes.
              </p>
              <Link
                to="/login"
                className="mt-6 inline-block text-sm text-brand-600 hover:underline"
              >
                Back to sign in
              </Link>
            </div>
          ) : (
            <>
              <h1 className="text-xl font-semibold text-gray-900">Reset your password</h1>
              <p className="mt-1 text-sm text-gray-600">
                Type the email you used to sign up. We'll send you a link.
              </p>
              <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
                <Input
                  label="Email"
                  type="email"
                  autoComplete="email"
                  error={errors.email?.message}
                  {...register('email')}
                />
                <Button type="submit" className="w-full" disabled={isSubmitting}>
                  {isSubmitting ? 'Sending…' : 'Send reset link'}
                </Button>
              </form>
              <p className="mt-6 text-center text-sm text-gray-600">
                Remembered it?{' '}
                <Link to="/login" className="text-brand-600 hover:underline">
                  Sign in
                </Link>
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
