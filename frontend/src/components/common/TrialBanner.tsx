/**
 * REQ-002 trial banner — shows on the dashboard while a Stripe subscription
 * is in `trialing` state. Calm tone: "Trial ends Sat May 23 — your card is
 * on file." Hides itself when the subscription is active or absent.
 */
import { useQuery } from '@tanstack/react-query';
import { Sparkles } from 'lucide-react';
import { Link } from 'react-router-dom';

import { billingApi } from '@/api/billing';

export default function TrialBanner(): JSX.Element | null {
  const { data: subscription } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => billingApi.getSubscription(),
    staleTime: 30_000,
  });

  if (!subscription) return null;
  if (subscription.status !== 'trialing') return null;

  // Prefer trialEndsAt (the precise stripe value) and fall back to
  // currentPeriodEnd if the webhook hasn't populated trial_ends_at yet.
  const endRaw = subscription.trialEndsAt ?? subscription.currentPeriodEnd;
  if (!endRaw) return null;
  const end = new Date(endRaw);
  if (Number.isNaN(end.getTime())) return null;
  const formatted = end.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });

  return (
    <div className="mb-4 flex items-center gap-3 rounded-xl border border-caution-100 bg-caution-50 px-4 py-3">
      <Sparkles className="h-5 w-5 shrink-0 text-caution-600" aria-hidden />
      <div className="flex-1 text-sm text-gray-800">
        <span className="font-medium">Trial ends {formatted}</span> — your card
        is on file.{' '}
        <Link
          to="/settings/billing"
          className="text-brand-600 hover:underline"
        >
          Manage billing
        </Link>
      </div>
    </div>
  );
}
