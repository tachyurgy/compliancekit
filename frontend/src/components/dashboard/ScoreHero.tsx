import { Link } from 'react-router-dom';
import { ArrowRight, Sparkles } from 'lucide-react';
import type { ScoreData, ScoreBand, GapAction } from '@/api/score';

// REQ-013 / REQ-012c — compliance-score hero. The single most important
// surface in the app: a tired Florida director should be able to glance at
// it and answer "am I OK?"
//
// Brand-voice law: coral, never red. Even at `urgent` the copy reads calm
// and prescriptive. No alarmist colour — sage at the top, caution amber in
// the middle, coral at the bottom. Cream-tinted card surface so the hero
// reads continuous with the body, not stamped onto it.

interface BandTokens {
  ring: string; // tailwind text-* class controlling the SVG stroke currentColor
  chip: string;
  chipText: string;
  surface: string;
  label: string;
  reassurance: string;
}

const BANDS: Record<ScoreBand, BandTokens> = {
  excellent: {
    ring: 'text-brand-600',
    chip: 'bg-brand-100',
    chipText: 'text-brand-800',
    surface: 'bg-brand-50/60 border-brand-100',
    label: 'Excellent',
    reassurance: "Nothing to chase today. We're still watching for you.",
  },
  solid: {
    ring: 'text-brand-500',
    chip: 'bg-brand-100',
    chipText: 'text-brand-800',
    surface: 'bg-brand-50/40 border-brand-100',
    label: 'Solid',
    reassurance: 'A handful of small things on the schedule.',
  },
  attention: {
    ring: 'text-caution-500',
    chip: 'bg-caution-100',
    chipText: 'text-caution-700',
    surface: 'bg-caution-50/70 border-caution-100',
    label: 'Needs attention',
    reassurance: "A few things this month — we've got the chase going.",
  },
  urgent: {
    ring: 'text-critical-600',
    chip: 'bg-critical-100',
    chipText: 'text-critical-700',
    surface: 'bg-critical-50/70 border-critical-100',
    label: 'On the chase',
    reassurance: "A few things to fix today and you're back to solid. We're on the chase.",
  },
};

function ScoreNumeral({ score, band }: { score: number; band: ScoreBand }): JSX.Element {
  const tokens = BANDS[band];
  const radius = 60;
  const circ = 2 * Math.PI * radius;
  const offset = circ - (score / 100) * circ;
  return (
    <div className="relative h-40 w-40 flex-shrink-0">
      <svg viewBox="0 0 144 144" className="h-full w-full -rotate-90">
        <circle
          cx="72"
          cy="72"
          r={radius}
          stroke="currentColor"
          strokeWidth="9"
          fill="none"
          className="text-line"
        />
        <circle
          cx="72"
          cy="72"
          r={radius}
          stroke="currentColor"
          strokeWidth="9"
          fill="none"
          strokeLinecap="round"
          strokeDasharray={circ}
          strokeDashoffset={offset}
          className={`${tokens.ring} transition-[stroke-dashoffset] duration-700`}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <div className="font-display text-[56px] leading-none font-semibold tracking-tight text-ink-900">
          {score}
        </div>
        <div className="text-[11px] uppercase tracking-[0.14em] text-ink-500 mt-1.5">of 100</div>
      </div>
    </div>
  );
}

function GapCard({ gap }: { gap: GapAction }): JSX.Element {
  return (
    <Link
      to={gap.cta_route}
      className="group block rounded-lb border border-line bg-white p-4 hover:border-brand-200 hover:shadow-card transition-all"
    >
      <div className="text-[11px] uppercase tracking-[0.12em] text-warm-600 font-semibold mb-1.5">
        Today
      </div>
      <div className="text-sm font-medium text-ink-900 leading-snug">{gap.title}</div>
      {gap.subtitle ? (
        <div className="mt-1 text-xs text-ink-700 leading-snug">{gap.subtitle}</div>
      ) : null}
      <div className="mt-3 inline-flex items-center gap-1 text-xs font-semibold text-brand-700 group-hover:text-brand-800">
        {gap.cta_label}
        <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
      </div>
    </Link>
  );
}

export default function ScoreHero({ data }: { data: ScoreData }): JSX.Element {
  const tokens = BANDS[data.band];
  const isPerfect = data.score === 100;

  return (
    <div
      data-tour-id="score-hero"
      className={`rounded-lb-lg border ${tokens.surface} p-6 sm:p-8`}
    >
      <div className="flex flex-col sm:flex-row sm:items-center gap-6">
        <ScoreNumeral score={data.score} band={data.band} />
        <div className="flex-1">
          <span
            className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold ${tokens.chip} ${tokens.chipText}`}
          >
            {isPerfect && <Sparkles className="h-3 w-3" aria-hidden />}
            {tokens.label}
          </span>
          <h2 className="mt-3 font-display text-2xl sm:text-3xl text-ink-900 leading-tight">
            {isPerfect
              ? "You're at 100. Nothing to chase today."
              : `You're at ${data.score}. ${tokens.reassurance}`}
          </h2>
          <p className="mt-2 text-sm text-ink-700 leading-relaxed">
            {data.top_gaps.length > 0
              ? "We're still watching for you. Tap a card below to fast-forward."
              : "We're still watching for you."}
          </p>
        </div>
      </div>

      {data.top_gaps.length > 0 ? (
        <div className="mt-7 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
          {data.top_gaps.slice(0, 4).map((gap, i) => (
            <div key={`${gap.kind}-${i}`} data-tour-id={i === 0 ? 'top-gap' : undefined}>
              <GapCard gap={gap} />
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-6 rounded-lb border border-brand-100 bg-white/70 p-4 text-sm text-brand-800 leading-relaxed">
          Nothing on the chase list right now. We'll surface anything that needs attention here.
        </div>
      )}

      {data.breakdown.length > 0 ? (
        <details className="mt-5 group">
          <summary className="cursor-pointer text-xs font-medium text-ink-700 hover:text-ink-900 inline-flex items-center gap-1.5">
            Why isn't it 100?
            <ArrowRight className="h-3 w-3 transition-transform group-open:rotate-90" />
          </summary>
          <div className="mt-3 space-y-1.5 text-xs text-ink-700">
            {data.breakdown.map((row) => (
              <div key={row.category} className="flex items-center justify-between">
                <span>
                  {row.category} <span className="text-ink-400">({row.count})</span>
                </span>
                <span className="font-mono font-medium text-ink-700">{row.deduction}</span>
              </div>
            ))}
          </div>
        </details>
      ) : null}
    </div>
  );
}

export function ScoreHeroSkeleton(): JSX.Element {
  return (
    <div className="rounded-lb-lg border border-line bg-brand-50/30 p-6 sm:p-8 animate-pulse">
      <div className="flex flex-col sm:flex-row sm:items-center gap-6">
        <div className="h-40 w-40 rounded-full bg-line-soft" />
        <div className="flex-1 space-y-3">
          <div className="h-5 w-24 bg-line-soft rounded-full" />
          <div className="h-7 w-2/3 bg-line-soft rounded" />
          <div className="h-4 w-1/2 bg-line-soft rounded" />
        </div>
      </div>
      <div className="mt-7 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-24 rounded-lb bg-line-soft" />
        ))}
      </div>
    </div>
  );
}
