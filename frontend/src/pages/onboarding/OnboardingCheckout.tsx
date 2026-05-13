import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { AlertCircle, Lock } from 'lucide-react';

import Button from '@/components/common/Button';
import BrandMark from '@/components/common/BrandMark';
import { billingApi, type CheckoutPlan } from '@/api/billing';
import { ApiError } from '@/api/client';

/**
 * REQ-002 #3: choose annual ($948/yr) — default — vs monthly ($99/mo).
 * Single CTA: "Start trial — no charge today" → Stripe Checkout.
 * Success URL on the backend is /onboarding/legal.
 */
export default function OnboardingCheckout(): JSX.Element {
  const [searchParams] = useSearchParams();
  const canceled = searchParams.get('canceled') === '1';
  const [plan, setPlan] = useState<CheckoutPlan>('annual_948');
  const [error, setError] = useState<string | null>(null);

  const checkout = useMutation({
    mutationFn: () => billingApi.createCheckoutSession(plan),
    onSuccess: ({ url }) => {
      window.location.assign(url);
    },
    onError: (err) => {
      setError(
        err instanceof ApiError
          ? err.message
          : "We couldn't start your checkout. Try again in a moment.",
      );
    },
  });

  return (
    <div className="min-h-screen bg-surface-muted">
      <header className="border-b border-line bg-white">
        <div className="max-w-5xl mx-auto px-6 h-16 flex items-center">
          <BrandMark to="/" size="md" />
        </div>
      </header>

      <main className="max-w-2xl mx-auto px-6 py-14">
        <div className="text-center">
          <span className="eyebrow">Step 1 of 3 — pick your plan</span>
          <h1 className="mt-4 font-display text-4xl text-ink-900 tracking-tight">
            Pick a plan.
          </h1>
          <p className="mt-3 text-base text-ink-700 leading-relaxed">
            14 days free, no charge today. Cancel any time.
          </p>
        </div>

        {canceled && (
          <div
            className="mt-7 rounded-lb-sm bg-caution-50 border border-caution-100 p-4 flex items-start gap-3"
            role="status"
          >
            <AlertCircle className="h-5 w-5 text-caution-700 mt-0.5 shrink-0" />
            <p className="text-sm text-ink-800 leading-relaxed">
              No charge — you canceled before adding your card. Pick a plan
              below to start your trial.
            </p>
          </div>
        )}

        <div className="mt-9 space-y-3">
          <PlanOption
            id="annual_948"
            selected={plan === 'annual_948'}
            onSelect={() => setPlan('annual_948')}
            badge="Save ~$240/yr"
            heading="Annual — $948/yr"
            sub="Billed once a year. About $79/mo."
          />
          <PlanOption
            id="monthly_99"
            selected={plan === 'monthly_99'}
            onSelect={() => setPlan('monthly_99')}
            heading="Monthly — $99/mo"
            sub="Billed every month. Same features, more flexibility."
          />
        </div>

        {error && (
          <p
            className="mt-5 rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
            role="alert"
          >
            {error}
          </p>
        )}

        <div className="mt-9">
          <Button
            fullWidth
            size="lg"
            loading={checkout.isPending}
            onClick={() => checkout.mutate()}
          >
            Start trial — no charge today
          </Button>
          <p className="mt-3 text-xs text-center text-ink-500 inline-flex items-center justify-center gap-1.5 w-full">
            <Lock className="h-3 w-3" aria-hidden />
            Stripe handles your card. We never see the number.
          </p>
        </div>

        <div className="mt-12 pt-7 border-t border-line text-center">
          <p className="text-sm text-ink-700">
            Multi-site or 100+ kids?{' '}
            <a
              href="mailto:levelbrookteam@gmail.com"
              className="font-medium text-brand-700 hover:underline"
            >
              levelbrookteam@gmail.com
            </a>
          </p>
        </div>
      </main>
    </div>
  );
}

interface PlanOptionProps {
  id: CheckoutPlan;
  selected: boolean;
  onSelect: () => void;
  badge?: string;
  heading: string;
  sub: string;
}

function PlanOption({
  id,
  selected,
  onSelect,
  badge,
  heading,
  sub,
}: PlanOptionProps): JSX.Element {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`w-full text-left rounded-lb border-2 p-5 transition-all ${
        selected
          ? 'border-brand-600 bg-brand-50/40 shadow-card'
          : 'border-line bg-white hover:border-brand-200 hover:shadow-card'
      }`}
      aria-pressed={selected}
    >
      <div className="flex items-start gap-4">
        <span
          className={`mt-1 h-5 w-5 shrink-0 rounded-full border-2 grid place-items-center ${
            selected ? 'border-brand-600 bg-brand-600' : 'border-line bg-white'
          }`}
          aria-hidden
        >
          {selected && <span className="h-1.5 w-1.5 rounded-full bg-white" />}
        </span>
        <div className="flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-lg font-semibold text-ink-900">{heading}</span>
            {badge && (
              <span className="inline-block text-xs font-semibold px-2 py-0.5 rounded-full bg-warm-50 text-warm-600 ring-1 ring-warm-100">
                {badge}
              </span>
            )}
          </div>
          <p className="mt-1 text-sm text-ink-700">{sub}</p>
        </div>
      </div>
      <input type="radio" name="plan" value={id} checked={selected} readOnly hidden />
    </button>
  );
}
