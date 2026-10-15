/**
 * Public-demo build flag. Set VITE_DEMO_MODE=1 at build time for the hosted
 * demo: the UI then offers a one-click sandbox and hides paid signup (the demo
 * server has Stripe disabled).
 */
export const DEMO_MODE: boolean = import.meta.env.VITE_DEMO_MODE === '1';

export const ARCHITECTURE_URL =
  'https://github.com/tachyurgy/compliancekit/blob/main/ARCHITECTURE.md';
