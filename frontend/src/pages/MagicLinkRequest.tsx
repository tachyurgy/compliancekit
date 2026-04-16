import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { ShieldCheck, Mail, ArrowRight } from 'lucide-react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import { emailSchema } from '@/lib/validation';
import { zodResolver } from '@/lib/zodResolver';
import { providersApi } from '@/api/providers';
import { ApiError } from '@/api/client';

const FormSchema = z.object({ email: emailSchema });
type FormValues = z.infer<typeof FormSchema>;

export default function MagicLinkRequest(): JSX.Element {
  const [sent, setSent] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(FormSchema) });

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null);
    try {
      await providersApi.requestMagicLink(values.email);
      setSent(values.email);
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
          <span className="font-semibold text-lg">ComplianceKit</span>
        </Link>
        <div className="card p-6">
          {sent ? (
            <div>
              <div className="mx-auto mb-4 h-12 w-12 rounded-full bg-brand-50 grid place-items-center">
                <Mail className="h-6 w-6 text-brand-600" />
              </div>
              <h1 className="text-xl font-semibold text-gray-900 text-center">
                Check your email
              </h1>
              <p className="mt-2 text-sm text-gray-600 text-center">
                We sent a sign-in link to <span className="font-medium">{sent}</span>. It
                expires in 15 minutes.
              </p>
              <div className="mt-6 flex justify-center">
                <Button variant="ghost" onClick={() => setSent(null)}>
                  Use a different email
                </Button>
              </div>
            </div>
          ) : (
            <>
              <h1 className="text-xl font-semibold text-gray-900">Sign in to ComplianceKit</h1>
              <p className="mt-1 text-sm text-gray-600">
                Enter your email and we'll send you a secure sign-in link. No passwords.
              </p>
              <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
                <Input
                  label="Work email"
                  type="email"
                  autoComplete="email"
                  autoFocus
                  placeholder="you@yourdaycare.com"
                  leftIcon={<Mail className="h-4 w-4" />}
                  error={errors.email?.message}
                  {...register('email')}
                />
                {submitError && <p className="text-sm text-critical-600">{submitError}</p>}
                <Button
                  type="submit"
                  fullWidth
                  loading={isSubmitting}
                  rightIcon={<ArrowRight className="h-4 w-4" />}
                >
                  Send me a link
                </Button>
              </form>
            </>
          )}
        </div>
        <p className="mt-4 text-xs text-gray-500 text-center">
          By signing in you agree to our terms of service and privacy policy.
        </p>
      </div>
    </div>
  );
}
