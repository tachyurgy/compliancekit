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

const FormSchema = z.object({
  name: z.string().min(1, 'Center name is required').max(200),
  email: emailSchema,
  stateCode: z.enum(['CA', 'TX', 'FL'], {
    errorMap: () => ({ message: 'Pick a state we support today' }),
  }),
  password: z
    .string()
    .min(10, 'Password must be at least 10 characters')
    .max(200),
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
        name: values.name,
        ownerEmail: values.email,
        stateCode: values.stateCode,
        password: values.password,
      });
      const me = await providersApi.me();
      if (me) setSession(me);
      navigate(me && !me.onboardingComplete ? '/onboarding' : '/dashboard', {
        replace: true,
      });
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
            Start your 14-day free trial. No credit card required.
          </p>

          <form onSubmit={onSubmit} className="mt-5 space-y-4" noValidate>
            <Input
              label="Center name"
              autoComplete="organization"
              error={errors.name?.message}
              {...register('name')}
            />
            <Input
              label="Owner email"
              type="email"
              autoComplete="email"
              error={errors.email?.message}
              {...register('email')}
            />
            <div>
              <label className="block text-sm font-medium text-gray-700">State</label>
              <select
                className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-brand-500 focus:ring-brand-500 text-sm"
                {...register('stateCode')}
                defaultValue=""
              >
                <option value="" disabled>
                  Pick a state
                </option>
                <option value="FL">Florida</option>
                <option value="TX">Texas</option>
                <option value="CA">California</option>
              </select>
              {errors.stateCode ? (
                <p className="mt-1 text-sm text-red-600">{errors.stateCode.message}</p>
              ) : null}
            </div>
            <Input
              label="Password"
              type="password"
              autoComplete="new-password"
              error={errors.password?.message}
              hint="At least 10 characters."
              {...register('password')}
            />

            {submitError ? (
              <p className="text-sm text-red-600" role="alert">
                {submitError}
              </p>
            ) : null}

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? 'Creating account…' : 'Create account'}
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
