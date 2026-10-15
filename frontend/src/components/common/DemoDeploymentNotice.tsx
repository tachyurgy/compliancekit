import { Info } from 'lucide-react';

import { DEMO_MODE } from '@/lib/demo';

export const DEMO_EMAIL = 'demo@example.com';
export const DEMO_PASSWORD = 'inspection-ready';

/**
 * Slim, site-wide strip shown on every page of the hosted demo build
 * (VITE_DEMO_MODE=1). It states plainly that this is a demo deployment with
 * fictional data, and repeats the shared login so a reviewer never has to
 * hunt for it. Renders nothing in a normal build.
 */
export default function DemoDeploymentNotice(): JSX.Element | null {
  if (!DEMO_MODE) return null;
  return (
    <div
      role="note"
      aria-label="Demo deployment notice"
      className="w-full bg-ink-900 text-white/90 text-[13px] leading-snug"
    >
      <div className="max-w-6xl mx-auto px-4 sm:px-6 py-2 flex items-start sm:items-center justify-center gap-2 text-center">
        <Info className="h-3.5 w-3.5 mt-0.5 sm:mt-0 shrink-0 text-brand-200" aria-hidden="true" />
        <p>
          <span className="font-semibold text-white">Demo deployment:</span>{' '}
          fictional data, sandboxes reset after 24h. Sign in with{' '}
          <code className="font-mono text-white">{DEMO_EMAIL}</code> /{' '}
          <code className="font-mono text-white">{DEMO_PASSWORD}</code>.
        </p>
      </div>
    </div>
  );
}
