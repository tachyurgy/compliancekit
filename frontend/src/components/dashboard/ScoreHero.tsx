import { Link } from 'react-router-dom';
import { ArrowRight } from 'lucide-react';
import type { ScoreData, ScoreBand, GapAction } from '@/api/score';

// REQ-013 hero card.  The single most important surface in the app: a tired
// Florida director should be able to glance at it and answer "am I OK?"
//
// Brand voice law (active/product/brand-voice.md): coral, never red. Even at
// `urgent` the copy reads calm and prescriptive. No exclamation marks beyond
// the empty-state celebration.

const BAND_TOKENS: Record<ScoreBand, { ring: string; text: string; bg: string; chip: string; chipText: string; label: string }> = {
  excellent: { ring: 'stroke-sage-500', text: 'text-sage-700', bg: 'bg-sage-50', chip: 'bg-sage-100', chipText: 'text-sage-800', label: 'Excellent' },
  solid:     { ring: 'stroke-sage-500', text: 'text-sage-700', bg: 'bg-sage-50', chip: 'bg-sage-100', chipText: 'text-sage-800', label: 'Solid' },
  attention: { ring: 'stroke-amber-500', text: 'text-amber-700', bg: 'bg-amber-50', chip: 'bg-amber-100', chipText: 'text-amber-800', label: 'Needs attention' },
  urgent:    { ring: 'stroke-coral-500', text: 'text-coral-700', bg: 'bg-coral-50', chip: 'bg-coral-100', chipText: 'text-coral-800', label: 'A few things to fix' },
};

// Tailwind in this project uses `caution` / `critical` for amber/red and `brand` for the warm gold.
// Map the band tokens to whatever palette the tailwind config actually exposes; we fall back to
// arbitrary value classes for the calm sage / coral tones.
const FALLBACK_TOKENS: Record<ScoreBand, { ring: string; text: string; bg: string; chip: string; chipText: string; label: string; reassurance: string }> = {
  excellent: { ring: 'text-emerald-500', text: 'text-emerald-700', bg: 'bg-emerald-50', chip: 'bg-emerald-100', chipText: 'text-emerald-800', label: 'Excellent', reassurance: "You're at 100. Nothing to chase today. We're still watching for you." },
  solid:     { ring: 'text-emerald-500', text: 'text-emerald-700', bg: 'bg-emerald-50', chip: 'bg-emerald-100', chipText: 'text-emerald-800', label: 'Solid',     reassurance: "Solid. A handful of small things on the schedule." },
  attention: { ring: 'text-amber-500',   text: 'text-amber-700',   bg: 'bg-amber-50',   chip: 'bg-amber-100',   chipText: 'text-amber-800',   label: 'Attention',  reassurance: "A few things need attention this month. We've got the chase going." },
  urgent:    { ring: 'text-orange-500',  text: 'text-orange-700',  bg: 'bg-orange-50',  chip: 'bg-orange-100',  chipText: 'text-orange-800',  label: 'On the chase',  reassurance: "Four things to fix today and you're back to solid. We're on the chase." },
};

void BAND_TOKENS;

function ScoreNumeral({ score, band }: { score: number; band: ScoreBand }) {
  const tokens = FALLBACK_TOKENS[band];
  const radius = 60;
  const circ = 2 * Math.PI * radius;
  const offset = circ - (score / 100) * circ;
  return (
    <div className="relative h-40 w-40 flex-shrink-0">
      <svg viewBox="0 0 144 144" className="h-full w-full -rotate-90">
        <circle cx="72" cy="72" r={radius} stroke="currentColor" strokeWidth="9" fill="none" className="text-gray-100" />
        <circle
          cx="72" cy="72" r={radius}
          stroke="currentColor" strokeWidth="9" fill="none" strokeLinecap="round"
          strokeDasharray={circ} strokeDashoffset={offset}
          className={tokens.ring}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <div className="text-5xl font-semibold tracking-tight text-gray-900" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>
          {score}
        </div>
        <div className="text-[11px] uppercase tracking-wider text-gray-500 mt-1">of 100</div>
      </div>
    </div>
  );
}

function GapCard({ gap }: { gap: GapAction }) {
  return (
    <Link
      to={gap.cta_route}
      className="group block rounded-xl border border-gray-200 bg-white p-4 hover:border-amber-400 hover:shadow-sm transition"
    >
      <div className="text-[11px] uppercase tracking-wider text-gray-500 mb-1">Today</div>
      <div className="text-sm font-medium text-gray-900 leading-snug">{gap.title}</div>
      {gap.subtitle ? (
        <div className="mt-1 text-xs text-gray-600 leading-snug">{gap.subtitle}</div>
      ) : null}
      <div className="mt-3 inline-flex items-center gap-1 text-xs font-medium text-amber-700 group-hover:text-amber-800">
        {gap.cta_label}
        <ArrowRight className="h-3 w-3" />
      </div>
    </Link>
  );
}

export default function ScoreHero({ data }: { data: ScoreData }) {
  const tokens = FALLBACK_TOKENS[data.band];
  const isPerfect = data.score === 100;

  return (
    <div className={`rounded-2xl border border-gray-200 ${tokens.bg} p-6 sm:p-8`}>
      <div className="flex flex-col sm:flex-row sm:items-center gap-6">
        <ScoreNumeral score={data.score} band={data.band} />
        <div className="flex-1">
          <div className={`inline-flex items-center px-2.5 py-1 rounded-full ${tokens.chip} ${tokens.chipText} text-xs font-semibold`}>
            {tokens.label}
          </div>
          <h2
            className="mt-3 text-2xl sm:text-3xl text-gray-900 leading-tight"
            style={{ fontFamily: 'Fraunces, Georgia, serif', fontWeight: 600, letterSpacing: '-0.01em' }}
          >
            {isPerfect
              ? "You're at 100. Nothing to chase today."
              : `You're at ${data.score}. ${tokens.reassurance.split('. ').slice(1).join('. ')}`}
          </h2>
          <p className="mt-2 text-sm text-gray-600">
            We're still watching for you. {data.top_gaps.length > 0 ? "Tap a card below to fast-forward." : ''}
          </p>
        </div>
      </div>

      {data.top_gaps.length > 0 ? (
        <div className="mt-6 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
          {data.top_gaps.slice(0, 4).map((gap, i) => (
            <GapCard key={`${gap.kind}-${i}`} gap={gap} />
          ))}
        </div>
      ) : (
        <div className="mt-6 rounded-xl border border-emerald-200 bg-white/60 p-4 text-sm text-emerald-800">
          ✓ Nothing on the chase list right now. We'll surface anything that needs attention here.
        </div>
      )}

      {data.breakdown.length > 0 ? (
        <details className="mt-4 group">
          <summary className="cursor-pointer text-xs text-gray-600 hover:text-gray-900 inline-flex items-center gap-1">
            Why isn't it 100?
            <ArrowRight className="h-3 w-3 transition group-open:rotate-90" />
          </summary>
          <div className="mt-3 space-y-1.5 text-xs text-gray-700">
            {data.breakdown.map((row) => (
              <div key={row.category} className="flex items-center justify-between">
                <span>{row.category} <span className="text-gray-400">({row.count})</span></span>
                <span className="font-mono font-medium text-gray-700">{row.deduction}</span>
              </div>
            ))}
          </div>
        </details>
      ) : null}
    </div>
  );
}

export function ScoreHeroSkeleton() {
  return (
    <div className="rounded-2xl border border-gray-200 bg-amber-50/40 p-6 sm:p-8 animate-pulse">
      <div className="flex flex-col sm:flex-row sm:items-center gap-6">
        <div className="h-40 w-40 rounded-full bg-gray-100" />
        <div className="flex-1 space-y-3">
          <div className="h-5 w-24 bg-gray-100 rounded-full" />
          <div className="h-7 w-2/3 bg-gray-100 rounded" />
          <div className="h-4 w-1/2 bg-gray-100 rounded" />
        </div>
      </div>
      <div className="mt-6 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-24 rounded-xl bg-gray-100" />
        ))}
      </div>
    </div>
  );
}
