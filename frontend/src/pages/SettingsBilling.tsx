import { Link } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  ArrowLeft,
  CreditCard,
  ExternalLink,
  AlertTriangle,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { billingApi } from '@/api/billing';
import { formatCentsAsUSD, formatDate } from '@/lib/format';
import { toast } from '@/components/common/Toast';
import { ApiError } from '@/api/client';

const PLANS = [
  {
    slug: 'starter' as const,
    name: 'Starter',
    price: '$49/mo',
    description: 'Solo owners and family child care homes.',
  },
  {
    slug: 'professional' as const,
    name: 'Professional',
    price: '$99/mo',
    description: 'Single-site centers up to 100 children.',
    highlight: true,
  },
  {
    slug: 'enterprise' as const,
    name: 'Enterprise',
    price: '$199/site/mo',
    description: 'Multi-site providers and franchise groups.',
  },
];

export default function SettingsBilling(): JSX.Element {
  const { data: subscription, isLoading, isError, refetch } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => billingApi.getSubscription(),
  });

  const checkoutMutation = useMutation({
    mutationFn: (plan: 'starter' | 'professional' | 'enterprise') =>
      billingApi.createCheckoutSession(plan),
    onSuccess: ({ url }) => {
      window.location.assign(url);
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Could not start checkout.'),
  });

  const portalMutation = useMutation({
    mutationFn: () => billingApi.createPortalSession(),
    onSuccess: ({ url }) => {
      window.location.assign(url);
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Could not open billing portal.'),
  });

  return (
    <div>
      <Link
        to="/settings"
        className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 mb-4"
      >
        <ArrowLeft className="h-4 w-4" /> Back to settings
      </Link>
      <PageHeader
        title="Billing"
        description="Your subscription and payment method."
        actions={
          subscription && (
            <Button
              variant="secondary"
              leftIcon={<CreditCard className="h-4 w-4" />}
              loading={portalMutation.isPending}
              onClick={() => portalMutation.mutate()}
              rightIcon={<ExternalLink className="h-4 w-4" />}
            >
              Open billing portal
            </Button>
          )
        }
      />

      {isLoading ? (
        <LoadingSpinner />
      ) : isError ? (
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load subscription</div>
              <Button className="mt-2" size="sm" variant="secondary" onClick={() => refetch()}>
                Retry
              </Button>
            </div>
          </div>
        </Card>
      ) : subscription ? (
        <Card padded>
          <div className="flex items-center justify-between">
            <div>
              <div className="text-xs text-gray-500 uppercase tracking-wide">
                Current plan
              </div>
              <div className="mt-1 text-lg font-semibold text-gray-900 capitalize">
                {subscription.plan}
              </div>
              <div className="mt-1 text-sm text-gray-600">
                {formatCentsAsUSD(subscription.priceCents)}/mo
                {subscription.currentPeriodEnd && (
                  <span>
                    {' '}
                    · Renews {formatDate(subscription.currentPeriodEnd)}
                  </span>
                )}
              </div>
            </div>
            <Badge
              variant={
                subscription.status === 'active' || subscription.status === 'trialing'
                  ? 'compliant'
                  : subscription.status === 'past_due'
                    ? 'critical'
                    : 'warning'
              }
            >
              {subscription.status}
            </Badge>
          </div>
        </Card>
      ) : (
        <Card padded>
          <div className="flex items-start gap-3">
            <Sparkles className="h-5 w-5 text-brand-600" />
            <div>
              <div className="font-medium text-gray-900">Start your free trial</div>
              <p className="text-sm text-gray-600">
                Pick a plan below. You won't be charged for 14 days.
              </p>
            </div>
          </div>
        </Card>
      )}

      <div className="mt-6 grid grid-cols-1 md:grid-cols-3 gap-4">
        {PLANS.map((plan) => (
          <div
            key={plan.slug}
            className={`card p-5 flex flex-col ${plan.highlight ? 'ring-2 ring-brand-600' : ''}`}
          >
            {plan.highlight && (
              <div className="inline-flex items-center gap-1 text-xs font-medium text-brand-700 mb-1">
                <ShieldCheck className="h-3.5 w-3.5" />
                Most popular
              </div>
            )}
            <div className="text-lg font-semibold text-gray-900">{plan.name}</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900">{plan.price}</div>
            <p className="mt-2 text-sm text-gray-600 flex-1">{plan.description}</p>
            <Button
              className="mt-4"
              variant={plan.highlight ? 'primary' : 'secondary'}
              loading={checkoutMutation.isPending && checkoutMutation.variables === plan.slug}
              onClick={() => checkoutMutation.mutate(plan.slug)}
            >
              {subscription ? 'Switch to this plan' : 'Start trial'}
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}
