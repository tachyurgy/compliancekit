import { useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  ChevronLeft,
  Download,
  Flag,
  MinusCircle,
  SkipForward,
  XCircle,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { inspectionsApi } from '@/api/inspections';
import type {
  Answer,
  DomainBreakdown,
  Item,
  RunDetail,
  Severity,
} from '@/api/inspections';
import { ApiError } from '@/api/client';
import { toast } from '@/components/common/Toast';

// --- helpers ---

function severityLabel(s: Severity): string {
  return s === 'critical' ? 'Critical' : s === 'major' ? 'Major' : 'Minor';
}
function severityVariant(s: Severity): 'critical' | 'warning' | 'neutral' {
  return s === 'critical' ? 'critical' : s === 'major' ? 'warning' : 'neutral';
}
function answerIcon(a: Answer | null): JSX.Element | null {
  if (a === 'pass') return <CheckCircle2 className="h-4 w-4 text-brand-600" />;
  if (a === 'fail') return <XCircle className="h-4 w-4 text-critical-600" />;
  if (a === 'na') return <MinusCircle className="h-4 w-4 text-gray-500" />;
  return null;
}

function scoreColor(score: number): string {
  if (score >= 90) return 'text-brand-600';
  if (score >= 70) return 'text-caution-600';
  return 'text-critical-600';
}

function domainOfItemIndex(detail: RunDetail, index: number): { name: string; position: number; total: number } {
  const domains = detail.checklist.domains;
  for (let i = 0; i < domains.length; i++) {
    const d = domains[i];
    const start = d.start_index;
    const end = start + d.item_count - 1;
    if (index >= start && index <= end) {
      return { name: d.name, position: i + 1, total: domains.length };
    }
  }
  return { name: '', position: 0, total: domains.length };
}

// --- summary sub-view (post-finalize) ---

function DomainBar({ d }: { d: DomainBreakdown }): JSX.Element {
  const denom = Math.max(1, d.total - d.na);
  const passPct = Math.round((d.passed / denom) * 100);
  const failPct = Math.round((d.failed / denom) * 100);
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-sm">
        <span className="font-medium text-gray-800">{d.name}</span>
        <span className="text-gray-500 text-xs">
          {d.passed} pass &middot; {d.failed} fail{d.na ? ` · ${d.na} N/A` : ''}
          {d.unanswered ? ` · ${d.unanswered} unanswered` : ''}
        </span>
      </div>
      <div className="flex h-2 rounded-full overflow-hidden bg-gray-100">
        <div className="bg-brand-500" style={{ width: `${passPct}%` }} />
        <div className="bg-critical-500" style={{ width: `${failPct}%` }} />
      </div>
    </div>
  );
}

function SummaryView({ detail }: { detail: RunDetail }): JSX.Element {
  const score = detail.run.score ?? 0;
  const citations = detail.predicted_citations ?? [];
  const domains = detail.domain_breakdown ?? [];

  const reportUrl = inspectionsApi.reportUrl(detail.run.id);

  return (
    <div className="space-y-6">
      <Card padded>
        <div className="flex flex-col sm:flex-row items-center gap-6">
          <div className="relative h-36 w-36 flex-shrink-0">
            <svg viewBox="0 0 128 128" className="h-full w-full -rotate-90">
              <circle
                cx="64"
                cy="64"
                r="54"
                stroke="currentColor"
                strokeWidth="10"
                fill="none"
                className="text-gray-100"
              />
              <circle
                cx="64"
                cy="64"
                r="54"
                stroke="currentColor"
                strokeWidth="10"
                fill="none"
                strokeLinecap="round"
                strokeDasharray={2 * Math.PI * 54}
                strokeDashoffset={2 * Math.PI * 54 - (score / 100) * 2 * Math.PI * 54}
                className={scoreColor(score)}
              />
            </svg>
            <div className="absolute inset-0 flex flex-col items-center justify-center">
              <div className="text-3xl font-semibold text-gray-900">{score}</div>
              <div className="text-xs text-gray-500">of 100</div>
            </div>
          </div>
          <div className="flex-1">
            <h2 className="text-xl font-semibold text-gray-900">Inspection complete</h2>
            <p className="text-sm text-gray-600 mt-1 max-w-lg">
              {score >= 90
                ? "You're inspection-ready. Fix the items below to lock in a perfect score."
                : score >= 70
                  ? 'You have real work to do before the state shows up. Prioritize the critical citations.'
                  : "This would not go well with a real inspector. Start with the critical items below."}
            </p>
            <div className="mt-3 flex flex-wrap gap-3 text-xs text-gray-600">
              <div>
                <span className="font-semibold text-gray-900">{detail.run.items_passed}</span> passed
              </div>
              <div>
                <span className="font-semibold text-gray-900">{detail.run.items_failed}</span> failed
              </div>
              <div>
                <span className="font-semibold text-gray-900">{detail.run.items_na}</span> N/A
              </div>
              <div>
                <span className="font-semibold text-gray-900">{detail.run.total_items}</span> total
              </div>
            </div>
            <div className="mt-4">
              <a href={reportUrl} target="_blank" rel="noreferrer">
                <Button leftIcon={<Download className="h-4 w-4" />} variant="secondary">
                  Export report as PDF
                </Button>
              </a>
            </div>
          </div>
        </div>
      </Card>

      <Card title="Domain breakdown" padded>
        <div className="space-y-4">
          {domains.map((d) => (
            <DomainBar key={d.name} d={d} />
          ))}
        </div>
      </Card>

      <Card
        title="Predicted citations"
        description="Every failed item — ranked by the severity an inspector would assign."
        padded
      >
        {citations.length === 0 ? (
          <div className="text-sm text-gray-600 flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4 text-brand-600" />
            No failed items. Nothing for an inspector to cite.
          </div>
        ) : (
          <ul className="divide-y divide-gray-100 -mx-5">
            {citations.map((c) => (
              <li key={c.item_id} className="px-5 py-3 flex items-start gap-3">
                <Flag className="h-4 w-4 mt-0.5 text-critical-600 flex-shrink-0" />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <Badge variant={severityVariant(c.severity)}>
                      {severityLabel(c.severity)}
                    </Badge>
                    <span className="text-xs text-gray-500">{c.domain}</span>
                  </div>
                  <div className="text-sm text-gray-900 mt-1">{c.question}</div>
                  <div className="text-xs text-gray-500 mt-1">
                    {c.reference} &middot; {c.form_ref}
                  </div>
                  {c.note ? (
                    <div className="text-xs text-gray-600 mt-1 italic">Note: {c.note}</div>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

// --- wizard sub-view (pre-finalize) ---

function Wizard({ detail }: { detail: RunDetail }): JSX.Element {
  const qc = useQueryClient();
  const [index, setIndex] = useState(0);
  const [noteDraft, setNoteDraft] = useState('');

  // Keep per-item local drafts aligned with response records when navigating.
  const items: Item[] = detail.checklist.items;
  const current = items[index];
  const responsesByItem = useMemo(() => {
    const m = new Map<string, { answer: Answer; note: string }>();
    for (const r of detail.responses) {
      m.set(r.item_id, { answer: r.answer, note: r.note || '' });
    }
    return m;
  }, [detail.responses]);

  const currentResponse = responsesByItem.get(current?.id ?? '');
  const answeredCount = detail.responses.length;
  const allAnswered = answeredCount === items.length;
  const domainInfo = domainOfItemIndex(detail, index);

  const answer = useMutation({
    mutationFn: async ({ id, a, note }: { id: string; a: Answer; note: string }) => {
      return inspectionsApi.answer(detail.run.id, id, {
        answer: a,
        note: note || undefined,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['inspection', detail.run.id] });
      void qc.invalidateQueries({ queryKey: ['inspections'] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : 'Could not save answer.');
    },
  });

  const finalize = useMutation({
    mutationFn: () => inspectionsApi.finalize(detail.run.id),
    onSuccess: () => {
      toast.success('Inspection finalized.');
      void qc.invalidateQueries({ queryKey: ['inspection', detail.run.id] });
      void qc.invalidateQueries({ queryKey: ['inspections'] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : 'Could not finalize.');
    },
  });

  if (!current) {
    return (
      <Card padded>
        <div className="text-sm text-gray-600">Checklist is empty for this state.</div>
      </Card>
    );
  }

  const recordAnswer = (a: Answer) => {
    answer.mutate(
      { id: current.id, a, note: noteDraft || currentResponse?.note || '' },
      {
        onSuccess: () => {
          setNoteDraft('');
          if (index < items.length - 1) setIndex(index + 1);
        },
      },
    );
  };

  const goPrev = () => {
    if (index > 0) {
      setNoteDraft('');
      setIndex(index - 1);
    }
  };
  const goSkip = () => {
    if (index < items.length - 1) {
      setNoteDraft('');
      setIndex(index + 1);
    }
  };

  const progressPct = Math.round((answeredCount / items.length) * 100);

  return (
    <div className="space-y-4">
      {/* Progress header */}
      <Card padded>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div>
            <div className="text-xs uppercase tracking-wide text-gray-500">
              Domain {domainInfo.position} of {domainInfo.total}
            </div>
            <div className="text-lg font-semibold text-gray-900">{domainInfo.name}</div>
          </div>
          <div className="text-right">
            <div className="text-xs text-gray-500">
              Question {index + 1} of {items.length}
            </div>
            <div className="text-sm text-gray-700">
              {answeredCount} answered &middot; {progressPct}% complete
            </div>
          </div>
        </div>
        <div className="mt-3 h-2 rounded-full bg-gray-100 overflow-hidden">
          <div
            className="h-2 bg-brand-500 transition-all"
            style={{ width: `${progressPct}%` }}
          />
        </div>
      </Card>

      {/* Current question */}
      <Card padded>
        <div className="flex items-center gap-2 flex-wrap mb-3">
          <Badge variant={severityVariant(current.severity)}>
            {severityLabel(current.severity)}
          </Badge>
          <span className="text-xs text-gray-500">{current.form_ref}</span>
          <span className="text-xs text-gray-400">&middot;</span>
          <span className="text-xs text-gray-500">{current.reference}</span>
        </div>

        <p className="text-lg sm:text-xl text-gray-900 leading-snug font-medium">
          {current.question}
        </p>

        {/* Big answer buttons — tablet-first */}
        <div className="mt-6 grid grid-cols-1 sm:grid-cols-3 gap-3">
          <Button
            size="lg"
            variant={currentResponse?.answer === 'pass' ? 'primary' : 'secondary'}
            fullWidth
            loading={answer.isPending}
            leftIcon={<CheckCircle2 className="h-5 w-5" />}
            onClick={() => recordAnswer('pass')}
          >
            Pass
          </Button>
          <Button
            size="lg"
            variant={currentResponse?.answer === 'fail' ? 'danger' : 'secondary'}
            fullWidth
            loading={answer.isPending}
            leftIcon={<XCircle className="h-5 w-5" />}
            onClick={() => recordAnswer('fail')}
          >
            Fail
          </Button>
          <Button
            size="lg"
            variant="secondary"
            fullWidth
            loading={answer.isPending}
            leftIcon={<MinusCircle className="h-5 w-5" />}
            onClick={() => recordAnswer('na')}
          >
            N/A
          </Button>
        </div>

        {/* Note + evidence */}
        <div className="mt-6">
          <label className="text-xs font-medium text-gray-700 uppercase tracking-wide">
            Note (optional)
          </label>
          <textarea
            className="mt-1 w-full rounded-md border border-gray-200 text-sm p-2 min-h-[72px]"
            placeholder={currentResponse?.note || 'Anything you want the inspector to see or you want to remember'}
            value={noteDraft}
            onChange={(e) => setNoteDraft(e.target.value)}
          />
          {currentResponse ? (
            <div className="mt-2 text-xs text-gray-500 flex items-center gap-2">
              {answerIcon(currentResponse.answer)}
              Currently answered <span className="font-medium">{currentResponse.answer.toUpperCase()}</span>
              {currentResponse.note ? ` — "${currentResponse.note}"` : ''}
            </div>
          ) : null}
        </div>
      </Card>

      {/* Navigation + finalize */}
      <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            leftIcon={<ChevronLeft className="h-4 w-4" />}
            onClick={goPrev}
            disabled={index === 0}
          >
            Prev
          </Button>
          <Button
            variant="ghost"
            leftIcon={<SkipForward className="h-4 w-4" />}
            onClick={goSkip}
            disabled={index >= items.length - 1}
          >
            Skip
          </Button>
        </div>
        <Button
          variant={allAnswered ? 'primary' : 'secondary'}
          loading={finalize.isPending}
          onClick={() => finalize.mutate()}
          rightIcon={<ArrowRight className="h-4 w-4" />}
        >
          {allAnswered ? 'Finalize inspection' : `Finalize (${answeredCount}/${items.length})`}
        </Button>
      </div>
    </div>
  );
}

// --- page shell ---

export default function InspectionDetail(): JSX.Element {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['inspection', id],
    queryFn: () => inspectionsApi.get(id),
    enabled: Boolean(id),
  });

  if (isLoading) {
    return (
      <div className="py-16 grid place-items-center">
        <LoadingSpinner label="Loading inspection…" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div>
        <PageHeader title="Inspection" />
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load this inspection</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Unknown error.'}
              </div>
              <div className="mt-3 flex gap-2">
                <Button size="sm" variant="secondary" onClick={() => refetch()}>
                  Retry
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate('/inspections')}>
                  Back
                </Button>
              </div>
            </div>
          </div>
        </Card>
      </div>
    );
  }

  const finalized = Boolean(data.run.completed_at);

  return (
    <div>
      <PageHeader
        eyebrow={data.checklist.form_ref}
        title={finalized ? 'Inspection summary' : 'Mock inspection'}
        description={
          finalized
            ? 'Finalized run. Export the report or start a new inspection from the list.'
            : 'One question at a time. Answer honestly — the score uses the same weighting the state uses.'
        }
        actions={
          <Button
            variant="ghost"
            leftIcon={<ArrowLeft className="h-4 w-4" />}
            onClick={() => navigate('/inspections')}
          >
            All inspections
          </Button>
        }
      />
      {finalized ? <SummaryView detail={data} /> : <Wizard detail={data} />}
    </div>
  );
}
