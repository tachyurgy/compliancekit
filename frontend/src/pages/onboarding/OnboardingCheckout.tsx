import { useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { ShieldCheck, AlertCircle } from 'lucide-react';

import Button from '@/components/common/Button';
import { billingApi, type CheckoutPlan } from '@/api/billing';
import { ApiError } from '@/api/client';

/**
 * REQ-002 #3: choose annual ($948/yr) — default — vs monthly ($99/mo).
 *
 * Single CTA: "Start trial — no charge today" → POST
 * /api/billing/checkout-session → window.location.assign(url) to Stripe
 * hosted Checkout. Success URL on the backend is /onboarding/legal.
 *
 * Visual notes (per active/product/brand-voice.md and CLAUDE.md):
 *   - annual is selected by default with a small "Save ~$240/yr" badge
 *   - monthly stays visible, never below the fold
 *   - enterprise hint is a plain mailto: link, no Calendly, no Contact Sales
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
      <header className="px-6 py-5 border-b border-gray-100 bg-white">
        <Link to="/" className="flex items-center gap-2">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold text-lg">Levelbrook</span>
        </Link>
      </header>

      <main className="max-w-2xl mx-auto px-6 py-12">
        <div className="text-center">
          <h1 className="text-3xl font-semibold text-gray-900">
            Pick your plan
          </h1>
          <p className="mt-2 text-base text-gray-600">
            14 days free, no charge today. Cancel any time.
          </p>
        </div>

        {canceled && (
          <div className="mt-6 rounded-lg bg-caution-50 border border-caution-100 p-4 flex items-start gap-3">
            <AlertCircle className="h-5 w-5 text-caution-600 mt-0.5" />
            <p className="text-sm text-gray-800">
              No charge — you canceled before adding your card. Pick a plan
              below to start your trial.
            </p>
          </div>
        )}

        <div className="mt-8 space-y-3">
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
          <p className="mt-4 text-sm text-red-600" role="alert">
            {error}
          </p>
        )}

        <div className="mt-8">
          <Button
            className="w-full"
            size="lg"
            loading={checkout.isPending}
            onClick={() => checkout.mutate()}
          >
            Start trial — no charge today
          </Button>
          <p className="mt-3 text-xs text-center text-gray-500">
            We use Stripe to handle payment. We never see your card number.
          </p>
        </div>

        <div className="mt-10 pt-6 border-t border-gray-100 text-center">
          <p className="text-sm text-gray-600">
            For multi-site or 100+ kids, email{' '}
            <a
              href="mailto:levelbrookteam@gmail.com"
              className="text-brand-600 hover:underline"
            >
              levelbrookteam@gmail.com
            </a>
            .
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
      className={`w-full text-left rounded-xl border-2 p-5 transition-colors ${
        selected
          ? 'border-brand-600 bg-brand-50/40 shadow-card'
          : 'border-gray-200 bg-white hover:border-gray-300'
      }`}
      aria-pressed={selected}
    >
      <div className="flex items-start gap-4">
        <span
          className={`mt-1 h-5 w-5 shrink-0 rounded-full border-2 ${
            selected
              ? 'border-brand-600 bg-brand-600'
              : 'border-gray-300 bg-white'
          }`}
          aria-hidden
        >
          {selected && (
            <span className="block h-full w-full rounded-full bg-white scale-50" />
          )}
        </span>
        <div className="flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-lg font-semibold text-gray-900">
              {heading}
            </span>
            {badge && (
              <span className="inline-block text-xs font-medium px-2 py-0.5 rounded-full bg-caution-100 text-caution-600">
                {badge}
              </span>
            )}
          </div>
          <p className="mt-1 text-sm text-gray-600">{sub}</p>
        </div>
      </div>
      <input type="radio" name="plan" value={id} checked={selected} readOnly hidden />
    </button>
  );
}
