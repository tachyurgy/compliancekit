import { z } from 'zod';
import { apiFetch } from './client';

export const SubscriptionSchema = z.object({
  plan: z.enum(['starter', 'professional', 'enterprise']),
  status: z.enum(['active', 'trialing', 'past_due', 'canceled', 'incomplete']),
  currentPeriodEnd: z.string().nullable(),
  cancelAtPeriodEnd: z.boolean(),
  priceCents: z.number().int(),
  seats: z.number().int().nullable(),
});
export type Subscription = z.infer<typeof SubscriptionSchema>;

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
  async createCheckoutSession(
    plan: 'starter' | 'professional' | 'enterprise',
  ): Promise<{ url: string }> {
    return apiFetch<{ url: string }>('/api/billing/checkout', {
      method: 'POST',
      json: { plan },
    });
  },
  async createPortalSession(): Promise<{ url: string }> {
    return apiFetch<{ url: string }>('/api/billing/portal', { method: 'POST' });
  },
};
