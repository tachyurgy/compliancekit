import type { ReactNode } from 'react';
import { ShieldCheck, Sparkles, Lock } from 'lucide-react';

import BrandMark from './BrandMark';
import DemoDeploymentNotice from './DemoDeploymentNotice';

interface AuthShellProps {
  title: string;
  description?: ReactNode;
  children: ReactNode;
  // Footer below the card — usually "Don't have an account? Create one".
  footer?: ReactNode;
}

// Two-pane shell for auth flows. Left pane = brand + reassurance copy
// (collapses on mobile); right pane = card with the actual form. Matches the
// marketing hero feel — warm radial gradient on a cream background — so the
// app feels like a continuation of the marketing site, not a different app.
export default function AuthShell({
  title,
  description,
  children,
  footer,
}: AuthShellProps): JSX.Element {
  return (
    <div
      className="min-h-screen bg-surface-muted"
      style={{
        backgroundImage:
          'radial-gradient(ellipse 90% 60% at 80% 0%, rgba(13,111,94,0.06), transparent 60%),' +
          'radial-gradient(ellipse 80% 50% at 0% 100%, rgba(197,119,74,0.06), transparent 60%)',
      }}
    >
      <DemoDeploymentNotice />
      <header className="max-w-6xl mx-auto px-6 h-16 flex items-center">
        <BrandMark to="/" size="md" />
      </header>

      <div className="max-w-6xl mx-auto px-6 pb-12 grid lg:grid-cols-[1fr_minmax(420px,520px)] gap-12 items-start">
        {/* Reassurance pane — desktop-only. Mirrors marketing hero copy: short,
            calm, with three trust signals. Hidden on mobile so the form leads. */}
        <aside className="hidden lg:block pt-8 pr-6 max-w-md">
          <span className="eyebrow">Florida Launch</span>
          <h2 className="mt-4 font-display text-4xl leading-[1.05] text-ink-900 tracking-tight">
            Inspection-ready, every single day.
          </h2>
          <p className="mt-5 text-base leading-relaxed text-ink-700">
            Levelbrook replaces the binder, the spreadsheet, and the
            pre-inspection panic with one dashboard your DCF inspector
            already knows how to read.
          </p>
          <ul className="mt-8 space-y-4 text-sm">
            <li className="flex items-start gap-3">
              <ShieldCheck className="h-5 w-5 text-brand-600 mt-0.5 shrink-0" />
              <span className="text-ink-800">
                Tracks every CF-FSP 5316 item the licensing counselor will ask for.
              </span>
            </li>
            <li className="flex items-start gap-3">
              <Sparkles className="h-5 w-5 text-warm-500 mt-0.5 shrink-0" />
              <span className="text-ink-800">
                14 days free. No charge today. Cancel any time.
              </span>
            </li>
            <li className="flex items-start gap-3">
              <Lock className="h-5 w-5 text-ink-500 mt-0.5 shrink-0" />
              <span className="text-ink-800">
                Your roster never leaves the U.S. Encrypted at rest, never sold.
              </span>
            </li>
          </ul>
        </aside>

        {/* Form pane */}
        <main className="pt-4 lg:pt-8 w-full">
          <div className="bg-white rounded-lb border border-line shadow-card p-7 sm:p-9">
            <h1 className="font-display text-2xl text-ink-900 tracking-tight">{title}</h1>
            {description && (
              <p className="mt-2 text-sm text-ink-700 leading-relaxed">{description}</p>
            )}
            <div className="mt-6">{children}</div>
          </div>
          {footer && (
            <div className="mt-6 text-center text-sm text-ink-700">{footer}</div>
          )}
        </main>
      </div>
    </div>
  );
}
