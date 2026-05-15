import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { ArrowRight, ClipboardList, Send, ShieldCheck, Sparkles, X } from 'lucide-react';

import ShaynaAvatar from './ShaynaAvatar';
import { demoApi } from '@/api/demo';
import { useShaynaTour } from '@/hooks/useShaynaTour';

// REQ-024 — Shayna's 4-beat post-checkout tour. Shape:
//
//   intro          full-screen modal, dismissible
//   score          floating panel + spotlight on [data-tour-id="score-hero"]
//   inspector_mode full-screen preview of Inspector Mode (no real navigation)
//   magic_link     floating panel with "Send the link" button + tick animation
//   finish         full-screen "Make this mine" CTA -> POST /api/demo/purge
//
// The component decides whether to render itself based on the hook state +
// whether the facility has demo data. We do NOT render anything if the
// operator has already finished the tour OR has no seeded data (eg they
// signed up before REQ-024 shipped). The Dashboard fetches /api/demo/status
// once and passes the boolean in.

interface Props {
  hasDemoData: boolean;
}

export default function ShaynaTour({ hasDemoData }: Props): JSX.Element | null {
  const tour = useShaynaTour();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [purging, setPurging] = useState(false);
  const [sent, setSent] = useState(false);

  // Spotlight overlay class management. We add `tour-spotlight` to the
  // current target element and a class to <body> so its z-index sits above
  // the dimmer.
  useEffect(() => {
    const targetSelector = TARGET_BY_STEP[tour.step];
    if (!targetSelector) return undefined;
    const el = document.querySelector<HTMLElement>(targetSelector);
    if (!el) return undefined;
    el.classList.add('tour-spotlight');
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    return () => {
      el.classList.remove('tour-spotlight');
    };
  }, [tour.step]);

  // Style block injected once. We don't have a global stylesheet hook for
  // arbitrary classes and pulling Tailwind through @layer for ephemeral
  // additions is more complexity than it's worth.
  useEffect(() => {
    if (document.getElementById('shayna-tour-style')) return;
    const tag = document.createElement('style');
    tag.id = 'shayna-tour-style';
    tag.textContent = `
      .tour-spotlight {
        position: relative;
        z-index: 60;
        border-radius: 18px;
        box-shadow:
          0 0 0 4px rgba(229, 161, 43, 0.85),
          0 0 0 9999px rgba(15, 23, 42, 0.55);
        animation: tour-pulse 2.4s ease-in-out infinite;
        transition: box-shadow 200ms ease;
      }
      @keyframes tour-pulse {
        0%, 100% { box-shadow:
          0 0 0 4px rgba(229, 161, 43, 0.85),
          0 0 0 9999px rgba(15, 23, 42, 0.55); }
        50%      { box-shadow:
          0 0 0 7px rgba(229, 161, 43, 0.6),
          0 0 0 9999px rgba(15, 23, 42, 0.55); }
      }
    `;
    document.head.appendChild(tag);
  }, []);

  if (!tour.isOpen) return null;
  if (!hasDemoData && tour.step !== 'finish') return null;

  if (tour.step === 'intro') {
    return (
      <ModalShell onSkip={tour.skip}>
        <div className="text-center">
          <div className="mx-auto">
            <ShaynaAvatar size={96} />
          </div>
          <h2 className="mt-5 font-display text-3xl text-ink-900 tracking-tight">
            Hi, I'm Shayna.
          </h2>
          <p className="mt-3 text-ink-700 text-base leading-relaxed max-w-md mx-auto">
            I run a center just like yours in Hialeah. I'll walk you through
            Levelbrook in about ninety seconds — your real account is in here,
            with a few example staff and kids so you can see what it'll look
            like once it's full. You can skip me any time.
          </p>
          <div className="mt-7 flex items-center justify-center gap-3">
            <PrimaryButton onClick={tour.next}>
              Yes, show me <ArrowRight className="h-4 w-4 ml-1" />
            </PrimaryButton>
            <SkipButton onClick={tour.skip}>I'll poke around</SkipButton>
          </div>
        </div>
      </ModalShell>
    );
  }

  if (tour.step === 'score') {
    return (
      <FloatingPanel onSkip={tour.skip}>
        <FloatingHeader step="Step 1 of 4" title="This is your score." />
        <p className="text-sm text-ink-700 leading-relaxed">
          One number, zero to a hundred. It's a stand-in for what your
          counselor sees walking in — the lower it is, the more they'll find.
          Tap any gap below it to fix it in one click.
        </p>
        <FloatingActions>
          <PrimaryButton onClick={tour.next}>
            Next: Inspector Mode <ArrowRight className="h-4 w-4 ml-1" />
          </PrimaryButton>
          <SkipButton onClick={tour.skip}>Skip the tour</SkipButton>
        </FloatingActions>
      </FloatingPanel>
    );
  }

  if (tour.step === 'inspector_mode') {
    return (
      <ModalShell onSkip={tour.skip}>
        <InspectorModePreview />
        <div className="mt-5 flex items-start gap-3 rounded-lb border border-warm-200 bg-warm-50 p-4">
          <ShaynaAvatar size={44} />
          <div className="flex-1">
            <div className="text-xs uppercase tracking-wide font-semibold text-warm-700">
              Step 2 of 4 · Inspector Mode
            </div>
            <p className="mt-1 text-sm text-ink-800 leading-relaxed">
              Hand the iPad to your counselor. That's it. No screenshots, no
              binder, no "let me find that file." This is what every
              competitor makes you build by hand.
            </p>
          </div>
        </div>
        <div className="mt-5 flex items-center justify-end gap-3">
          <SkipButton onClick={tour.skip}>Skip</SkipButton>
          <PrimaryButton onClick={tour.next}>
            Next: send a parent a link <ArrowRight className="h-4 w-4 ml-1" />
          </PrimaryButton>
        </div>
      </ModalShell>
    );
  }

  if (tour.step === 'magic_link') {
    return (
      <FloatingPanel onSkip={tour.skip}>
        <FloatingHeader step="Step 3 of 4" title="Close a gap in one tap." />
        <p className="text-sm text-ink-700 leading-relaxed">
          Liam Davis is missing his DH-680. Send his mom Karina a link — she
          opens it on her phone, snaps a photo at the pediatrician's office,
          and we file it for you.
        </p>
        <div className="mt-3 rounded-lb-sm border border-ink-200 bg-white p-3 text-sm">
          <div className="text-xs uppercase tracking-wide text-ink-500 font-semibold">
            To
          </div>
          <div className="mt-0.5 text-ink-900">
            Karina Davis · <span className="text-ink-500">karina@example.com</span>
          </div>
        </div>
        <FloatingActions>
          {!sent ? (
            <PrimaryButton onClick={() => setSent(true)}>
              <Send className="h-4 w-4 mr-1" /> Send the link
            </PrimaryButton>
          ) : (
            <PrimaryButton onClick={tour.next}>
              Score jumped 67 → 74. Next →
            </PrimaryButton>
          )}
          <SkipButton onClick={tour.skip}>Skip</SkipButton>
        </FloatingActions>
        {sent && (
          <div
            className="mt-3 flex items-center gap-2 text-sm text-good-700"
            role="status"
            aria-live="polite"
          >
            <span className="grid place-items-center h-5 w-5 rounded-full bg-good-100">
              <ShieldCheck className="h-3 w-3 text-good-600" />
            </span>
            Karina got the text. Filed under Liam's profile.
          </div>
        )}
      </FloatingPanel>
    );
  }

  // finish
  return (
    <ModalShell onSkip={tour.skip}>
      <div className="text-center">
        <div className="mx-auto inline-grid place-items-center h-14 w-14 rounded-full bg-warm-100 text-warm-700">
          <Sparkles className="h-7 w-7" />
        </div>
        <h2 className="mt-5 font-display text-3xl text-ink-900 tracking-tight">
          You've got this.
        </h2>
        <p className="mt-3 text-ink-700 text-base leading-relaxed max-w-md mx-auto">
          Click <strong>Make this mine</strong> and we'll clear the example
          data. Then add your first real staff member or child — we'll do
          the rest of the chasing for you.
        </p>
        <div className="mt-7 flex items-center justify-center gap-3">
          <PrimaryButton
            onClick={async () => {
              setPurging(true);
              try {
                await demoApi.purge();
                await Promise.allSettled([
                  qc.invalidateQueries({ queryKey: ['score'] }),
                  qc.invalidateQueries({ queryKey: ['dashboard'] }),
                  qc.invalidateQueries({ queryKey: ['facility'] }),
                  qc.invalidateQueries({ queryKey: ['children'] }),
                  qc.invalidateQueries({ queryKey: ['staff'] }),
                ]);
                tour.finish();
                navigate('/staff', { replace: false });
              } finally {
                setPurging(false);
              }
            }}
            disabled={purging}
          >
            {purging ? 'Clearing…' : 'Make this mine'}
            <ArrowRight className="h-4 w-4 ml-1" />
          </PrimaryButton>
          <SkipButton onClick={() => tour.finish()}>
            Keep poking around
          </SkipButton>
        </div>
        <p className="mt-5 text-xs text-ink-500">
          You can replay this tour any time at{' '}
          <code className="px-1.5 py-0.5 rounded bg-ink-100 text-ink-700">
            /dashboard?tour=replay
          </code>
          .
        </p>
      </div>
    </ModalShell>
  );
}

// ---------- which elements does each step spotlight ----------

const TARGET_BY_STEP: Partial<Record<string, string>> = {
  score: '[data-tour-id="score-hero"]',
  magic_link: '[data-tour-id="top-gap"]',
};

// ---------- visual primitives ----------

function ModalShell({
  children,
  onSkip,
}: {
  children: React.ReactNode;
  onSkip: () => void;
}): JSX.Element {
  return (
    <div
      className="fixed inset-0 z-[100] bg-ink-900/60 backdrop-blur-[2px] grid place-items-center p-6"
      role="dialog"
      aria-modal="true"
    >
      <div className="relative w-full max-w-xl rounded-lb-lg bg-white shadow-card p-8 border border-line">
        <button
          type="button"
          onClick={onSkip}
          aria-label="Close tour"
          className="absolute top-3 right-3 text-ink-400 hover:text-ink-700 p-1.5 rounded-md hover:bg-ink-50"
        >
          <X className="h-4 w-4" />
        </button>
        {children}
      </div>
    </div>
  );
}

function FloatingPanel({
  children,
  onSkip,
}: {
  children: React.ReactNode;
  onSkip: () => void;
}): JSX.Element {
  return (
    <div
      className="fixed bottom-6 right-6 z-[100] w-[400px] max-w-[calc(100vw-32px)] rounded-lb-lg bg-white shadow-card border border-warm-200 p-5"
      role="dialog"
      aria-modal="false"
    >
      <button
        type="button"
        onClick={onSkip}
        aria-label="Close tour"
        className="absolute top-2 right-2 text-ink-400 hover:text-ink-700 p-1 rounded-md hover:bg-ink-50"
      >
        <X className="h-4 w-4" />
      </button>
      <div className="flex items-start gap-3">
        <ShaynaAvatar size={48} />
        <div className="flex-1 min-w-0">{children}</div>
      </div>
    </div>
  );
}

function FloatingHeader({
  step,
  title,
}: {
  step: string;
  title: string;
}): JSX.Element {
  return (
    <>
      <div className="text-[11px] uppercase tracking-wide font-semibold text-warm-700">
        {step}
      </div>
      <div className="mt-0.5 text-base font-semibold text-ink-900">
        {title}
      </div>
    </>
  );
}

function FloatingActions({ children }: { children: React.ReactNode }): JSX.Element {
  return <div className="mt-4 flex items-center gap-3 flex-wrap">{children}</div>;
}

function PrimaryButton({
  onClick,
  children,
  disabled,
}: {
  onClick?: () => void | Promise<void>;
  children: React.ReactNode;
  disabled?: boolean;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="inline-flex items-center gap-1 px-3.5 py-2 rounded-lb-sm bg-ink-900 text-white text-sm font-semibold shadow-sm hover:bg-ink-800 active:bg-ink-900 disabled:opacity-60 disabled:cursor-not-allowed"
    >
      {children}
    </button>
  );
}

function SkipButton({
  onClick,
  children,
}: {
  onClick: () => void;
  children: React.ReactNode;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      className="text-sm text-ink-500 hover:text-ink-800 px-2 py-1"
    >
      {children}
    </button>
  );
}

// ---------- Inspector Mode preview ----------

function InspectorModePreview(): JSX.Element {
  return (
    <div className="overflow-hidden rounded-lb border border-ink-200 shadow-card">
      <div className="bg-[#0F172A] text-white p-5 pb-4">
        <div className="flex items-end justify-between">
          <div>
            <div className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-semibold">
              DCF Licensing Counselor View · CF-FSP 5316
            </div>
            <div className="mt-1 text-lg font-bold">Sunshine Kids of Hialeah</div>
            <div className="text-xs text-slate-300">
              On site: Monday · 9:00 AM · Counselor: M. Alvarez
            </div>
          </div>
          <div className="text-right">
            <div className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-semibold">
              Files current
            </div>
            <div className="text-2xl font-extrabold text-emerald-400">12 / 14</div>
          </div>
        </div>
      </div>
      <div className="bg-[#0B1220] text-slate-200 px-5 py-4 grid grid-cols-1 sm:grid-cols-2 gap-2 text-[13px]">
        <InspectorRow ok label="All staff first-aid certificates current" />
        <InspectorRow ok label="Background checks (Level 2) on file" />
        <InspectorRow warn label="CPR — 1 staff expires in 6 days (M. Vasquez)" />
        <InspectorRow warn label="DH-680 — 1 child missing (L. Davis)" />
        <InspectorRow ok label="Fire drill log — monthly through May" />
        <InspectorRow ok label="45-hr in-service tracking — 11 of 12 staff" />
      </div>
    </div>
  );
}

function InspectorRow({
  warn,
  label,
}: {
  ok?: boolean;
  warn?: boolean;
  label: string;
}): JSX.Element {
  return (
    <div
      className={`flex items-center gap-2 px-3 py-2 rounded ${
        warn ? 'bg-amber-950 text-amber-100' : 'bg-slate-800'
      }`}
    >
      <span className={warn ? 'text-amber-300' : 'text-emerald-400'}>
        {warn ? '!' : '✓'}
      </span>
      <span>{label}</span>
    </div>
  );
}

// Re-export the icon for callers that want to lazy-mount the tour later.
export { ClipboardList as TourIcon };
