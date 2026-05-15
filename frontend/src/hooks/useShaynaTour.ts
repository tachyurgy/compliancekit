import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

// REQ-024: state machine that drives the post-checkout onboarding tour
// hosted by Shayna. Tour state lives in localStorage so a tab refresh
// resumes mid-tour, and a ?tour=replay query string resets it (useful for
// support and for re-watching).
//
// Steps:
//   intro          — full-screen modal: "Hi, I'm Shayna."
//   score          — bubble next to the score hero
//   inspector_mode — full-screen preview of Inspector Mode (no nav)
//   magic_link     — bubble next to the top gap; "Send" tick animation
//   finish         — full-screen "Make this mine" CTA
//   done           — tour complete, never rendered again

export const TOUR_STEPS = [
  'intro',
  'score',
  'inspector_mode',
  'magic_link',
  'finish',
  'done',
] as const;
export type TourStep = (typeof TOUR_STEPS)[number];

const STORAGE_KEY = 'levelbrook-shayna-tour-state';

interface PersistedState {
  step: TourStep;
  // ISO timestamp of the last advance — useful in case we want to
  // re-prompt operators who abandoned the tour mid-way.
  updatedAt: string;
}

function readPersisted(): PersistedState | null {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<PersistedState>;
    if (!parsed.step || !TOUR_STEPS.includes(parsed.step)) return null;
    return { step: parsed.step, updatedAt: parsed.updatedAt ?? '' };
  } catch {
    return null;
  }
}

function writePersisted(step: TourStep): void {
  try {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ step, updatedAt: new Date().toISOString() }),
    );
  } catch {
    // Private mode etc — tour just won't persist. Acceptable.
  }
}

export interface ShaynaTour {
  step: TourStep;
  isOpen: boolean;
  start: () => void;
  next: () => void;
  skip: () => void;
  finish: () => void;
  replay: () => void;
  goTo: (step: TourStep) => void;
}

export function useShaynaTour(): ShaynaTour {
  const [params, setParams] = useSearchParams();
  const replayRequested = params.get('tour') === 'replay';

  const [step, setStep] = useState<TourStep>(() => {
    if (replayRequested) return 'intro';
    const persisted = readPersisted();
    return persisted?.step ?? 'intro';
  });

  // Consume ?tour=replay once: reset state, then strip the param so a
  // refresh doesn't loop the operator back to the intro.
  useEffect(() => {
    if (!replayRequested) return;
    writePersisted('intro');
    setStep('intro');
    const next = new URLSearchParams(params);
    next.delete('tour');
    setParams(next, { replace: true });
  }, [replayRequested, params, setParams]);

  const advance = useCallback(
    (next: TourStep) => {
      writePersisted(next);
      setStep(next);
    },
    [],
  );

  const order: TourStep[] = ['intro', 'score', 'inspector_mode', 'magic_link', 'finish', 'done'];

  return {
    step,
    isOpen: step !== 'done',
    start: () => advance('intro'),
    next: () => {
      const idx = order.indexOf(step);
      if (idx < 0 || idx >= order.length - 1) return;
      advance(order[idx + 1]);
    },
    skip: () => advance('done'),
    finish: () => advance('done'),
    replay: () => advance('intro'),
    goTo: (s) => advance(s),
  };
}
