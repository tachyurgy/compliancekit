import { z } from 'zod';
import { apiFetch } from './client';

/**
 * REQ-002 plan codes. The legacy values ('starter', 'professional') are
 * accepted by the schema for backward compatibility with the existing
 * SettingsBilling card layout, but new flows ship monthly_99 / annual_948.
 */
export const SubscriptionSchema = z.object({
  plan: z.enum([
    'starter',
    'professional',
    'enterprise',
    'monthly_99',
    'annual_948',
  ]),
  status: z.enum(['active', 'trialing', 'past_due', 'canceled', 'incomplete']),
  currentPeriodEnd: z.string().nullable(),
  cancelAtPeriodEnd: z.boolean().default(false),
  priceCents: z.number().int(),
  seats: z.number().int().nullable().default(null),
  trialEndsAt: z.string().nullable().optional(),
});
export type Subscription = z.infer<typeof SubscriptionSchema>;

export type CheckoutPlan = 'monthly_99' | 'annual_948';

export const billingApi = {
  async getSubscription(): Promise<Subscription | null> {
    try {
      const data = await apiFetch<unknown>('/api/billing/subscription');
      if (!data) return null;
      return SubscriptionSchema.parse(data);
    } catch {
      return null;
    }
  },
  /**
   * Legacy: kept for the older SettingsBilling card UI. New flows should use
   * `createCheckoutSession`.
   */
  async createCheckoutSessionLegacy(
    plan: 'starter' | 'professional' | 'enterprise',
  ): Promise<{ url: string }> {
    return apiFetch<{ url: string }>('/api/billing/checkout', {
      method: 'POST',
      json: { plan },
    });
  },
  /**
   * REQ-002 shape. Returns a Stripe-hosted Checkout URL with a 14-day trial,
   * card-on-file required, success → /onboarding/legal.
   */
  async createCheckoutSession(plan: CheckoutPlan): Promise<{ url: string }> {
    return apiFetch<{ url: string }>('/api/billing/checkout-session', {
      method: 'POST',
      json: { plan },
    });
  },
  async createPortalSession(): Promise<{ url: string }> {
    return apiFetch<{ url: string }>('/api/billing/portal-session', {
      method: 'POST',
    });
  },
};
