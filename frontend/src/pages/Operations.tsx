import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  ClipboardCheck,
  Flame,
  Plus,
  Trash2,
  Users,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Button from '@/components/common/Button';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Input from '@/components/common/Input';
import Select from '@/components/common/Select';
import Modal from '@/components/common/Modal';
import EmptyState from '@/components/common/EmptyState';
import { toast } from '@/components/common/Toast';
import EvidencePhotos from '@/components/EvidencePhotos';
import {
  operationsApi,
  type DrillKind,
  type DrillLog,
  type PostingItem,
  type RoomRatioStatus,
  DRILL_CADENCE_DAYS,
  drillLabel,
} from '@/api/operations';
import { formatDate, formatRelativeDays, isMeaningfulDate } from '@/lib/format';

// ---------------------------------------------------------------------------
// Page shell — tabs + routing-free sub-view switch.
// ---------------------------------------------------------------------------

type Tab = 'drills' | 'postings' | 'ratio';

const TABS: { id: Tab; label: string; icon: typeof Flame }[] = [
  { id: 'drills', label: 'Drills', icon: Flame },
  { id: 'postings', label: 'Postings', icon: ClipboardCheck },
  { id: 'ratio', label: 'Ratio', icon: Users },
];

export default function Operations(): JSX.Element {
  const [tab, setTab] = useState<Tab>('drills');
  return (
    <div>
      <PageHeader
        title="Facility & Operations"
        description="Log drills, track wall postings, and verify staff:child ratios. These feed directly into your compliance score."
      />

      <div className="mb-6 flex flex-wrap gap-2 border-b border-line-soft">
        {TABS.map((t) => {
          const Icon = t.icon;
          const active = tab === t.id;
          return (
            <button
              key={t.id}
              type="button"
              onClick={() => setTab(t.id)}
              className={`inline-flex items-center gap-2 px-4 py-2 -mb-px border-b-2 text-sm font-medium transition-colors ${
                active
                  ? 'border-brand-600 text-brand-700'
                  : 'border-transparent text-ink-700 hover:text-ink-900'
              }`}
            >
              <Icon className="h-4 w-4" />
              {t.label}
            </button>
          );
        })}
      </div>

      {tab === 'drills' && <DrillsTab />}
      {tab === 'postings' && <PostingsTab />}
      {tab === 'ratio' && <RatioTab />}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Drills tab
// ---------------------------------------------------------------------------

const DRILL_KINDS: DrillKind[] = [
  'fire',
  'tornado',
  'lockdown',
  'earthquake',
  'evacuation',
  'other',
];

function DrillsTab(): JSX.Element {
  const qc = useQueryClient();
  const [showModal, setShowModal] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const { data: drills = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['drills'],
    queryFn: () => operationsApi.listDrills(),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => operationsApi.deleteDrill(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['drills'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
      toast.success('Drill removed');
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Could not delete drill');
    },
  });

  // Compute "last X drill was N days ago; overdue?" per kind.
  // Ignore rows whose drill_date is the zero/sentinel value — those slipped
  // in through a backend bug; treating them as legitimate produces the
  // "739759 days ago" rendering.
  const lastByKind = useMemo(() => {
    const m = new Map<DrillKind, DrillLog>();
    for (const d of drills) {
      const dDate = new Date(d.drill_date);
      if (!isMeaningfulDate(dDate)) continue;
      const prev = m.get(d.drill_kind);
      if (!prev || dDate > new Date(prev.drill_date)) {
        m.set(d.drill_kind, d);
      }
    }
    return m;
  }, [drills]);

  return (
    <>
      <div className="flex justify-end mb-4">
        <Button
          leftIcon={<Plus className="h-4 w-4" />}
          onClick={() => setShowModal(true)}
        >
          Log a new drill
        </Button>
      </div>

      {/* Cadence summary */}
      <Card title="Cadence" padded className="mb-6">
        <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
          {DRILL_KINDS.map((kind) => {
            const last = lastByKind.get(kind);
            const cadence = DRILL_CADENCE_DAYS[kind];
            const overdue = last
              ? daysSince(last.drill_date) > cadence
              : true;
            return (
              <div
                key={kind}
                className={`rounded-lg border p-3 text-sm ${
                  overdue
                    ? 'border-critical-200 bg-critical-50'
                    : 'border-line-soft bg-white'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium capitalize">{kind}</span>
                  {overdue ? (
                    <Badge variant="critical">Overdue</Badge>
                  ) : (
                    <Badge variant="compliant">Current</Badge>
                  )}
                </div>
                <div className="mt-1 text-xs text-ink-700">
                  {last
                    ? `Last run ${formatRelativeDays(last.drill_date)} (${formatDate(last.drill_date)})`
                    : 'Never logged'}
                </div>
                <div className="mt-0.5 text-xs text-ink-500">
                  Expected cadence: every {cadence}d
                </div>
              </div>
            );
          })}
        </div>
      </Card>

      {isLoading ? (
        <Card padded>
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="skeleton h-10 w-full" />
            ))}
          </div>
        </Card>
      ) : isError ? (
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load drills</div>
              <Button
                size="sm"
                variant="secondary"
                className="mt-3"
                onClick={() => refetch()}
              >
                Retry
              </Button>
            </div>
          </div>
        </Card>
      ) : drills.length === 0 ? (
        <EmptyState
          icon={<Flame className="h-5 w-5 text-ink-500" />}
          title="No drills logged yet"
          description="Log your next fire or lockdown drill to start the cadence clock."
          action={
            <Button
              leftIcon={<Plus className="h-4 w-4" />}
              onClick={() => setShowModal(true)}
            >
              Log a new drill
            </Button>
          }
        />
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-line-soft text-xs uppercase tracking-wide text-ink-500">
                  <th className="px-5 py-3 font-medium">Kind</th>
                  <th className="px-5 py-3 font-medium">Date</th>
                  <th className="px-5 py-3 font-medium">Duration</th>
                  <th className="px-5 py-3 font-medium">Notes</th>
                  <th className="px-5 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {drills.map((d) => {
                  const isOpen = expandedId === d.id;
                  return (
                    <>
                      <tr key={d.id} className="hover:bg-surface-muted">
                        <td className="px-5 py-3 font-medium text-ink-900 capitalize">
                          {drillLabel(d.drill_kind)}
                        </td>
                        <td className="px-5 py-3 text-ink-700">
                          {formatDate(d.drill_date)}
                        </td>
                        <td className="px-5 py-3 text-ink-700">
                          {d.duration_seconds
                            ? formatDuration(d.duration_seconds)
                            : '—'}
                        </td>
                        <td className="px-5 py-3 text-ink-700 max-w-md truncate">
                          {d.notes || '—'}
                        </td>
                        <td className="px-5 py-3 text-right whitespace-nowrap">
                          <button
                            type="button"
                            onClick={() => setExpandedId(isOpen ? null : d.id)}
                            className="text-xs text-ink-600 hover:text-brand-700 mr-3"
                          >
                            {isOpen ? 'Hide photos' : 'Photos'}
                          </button>
                          <button
                            type="button"
                            aria-label="Delete drill"
                            onClick={() => {
                              if (confirm('Delete this drill log?')) {
                                deleteMutation.mutate(d.id);
                              }
                            }}
                            className="p-1 rounded text-ink-400 hover:text-critical-600 hover:bg-critical-50"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </td>
                      </tr>
                      {isOpen && (
                        <tr key={`${d.id}-photos`} className="bg-surface-muted">
                          <td colSpan={5} className="px-5 py-4">
                            <EvidencePhotos
                              subjectKind="drill"
                              subjectId={d.id}
                              title="Drill photos"
                              compact
                            />
                          </td>
                        </tr>
                      )}
                    </>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      <DrillModal open={showModal} onClose={() => setShowModal(false)} />
    </>
  );
}

function DrillModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}): JSX.Element {
  const qc = useQueryClient();
  const [kind, setKind] = useState<DrillKind>('fire');
  const [date, setDate] = useState<string>(() => new Date().toISOString().slice(0, 10));
  const [durationMinutes, setDurationMinutes] = useState<string>('');
  const [notes, setNotes] = useState('');

  // Reset form whenever the modal re-opens.
  useEffect(() => {
    if (open) {
      setKind('fire');
      setDate(new Date().toISOString().slice(0, 10));
      setDurationMinutes('');
      setNotes('');
    }
  }, [open]);

  const createMutation = useMutation({
    mutationFn: () =>
      operationsApi.createDrill({
        drill_kind: kind,
        drill_date: new Date(date).toISOString(),
        duration_seconds: durationMinutes
          ? Math.round(parseFloat(durationMinutes) * 60)
          : undefined,
        notes: notes || undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['drills'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
      toast.success('Drill logged');
      onClose();
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Could not save drill');
    },
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Log a drill"
      description="Record what was practiced, when, and how long it took. Attach photos as evidence below."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            loading={createMutation.isPending}
            onClick={() => createMutation.mutate()}
          >
            Save drill
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Select
          label="Drill kind"
          value={kind}
          onChange={(e) => setKind(e.target.value as DrillKind)}
          options={DRILL_KINDS.map((k) => ({
            value: k,
            label: drillLabel(k),
          }))}
        />
        <Input
          label="Date"
          type="date"
          value={date}
          onChange={(e) => setDate(e.target.value)}
        />
        <Input
          label="Duration (minutes)"
          type="number"
          min={0}
          step={0.5}
          placeholder="e.g. 3"
          value={durationMinutes}
          onChange={(e) => setDurationMinutes(e.target.value)}
          hint="Optional — from alarm start to all-clear."
        />
        <div>
          <label className="block text-sm font-medium text-ink-800 mb-1">
            Notes
          </label>
          <textarea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            rows={3}
            className="block w-full rounded-lg border border-line bg-white text-sm px-3 py-2 focus:outline-none focus:ring-2 focus:ring-brand-600 focus:ring-opacity-20 focus:border-brand-600"
            placeholder="Who led it, which room, anything that didn't go to plan."
          />
        </div>
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Postings tab
// ---------------------------------------------------------------------------

function PostingsTab(): JSX.Element {
  const qc = useQueryClient();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['postings'],
    queryFn: () => operationsApi.getPostings(),
  });

  const upsert = useMutation({
    mutationFn: (args: {
      key: string;
      input: { posted_at?: string; photo_document_id?: string; unpost?: boolean };
    }) => operationsApi.upsertPosting(args.key, args.input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['postings'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Could not update posting');
    },
  });

  const items = data?.items ?? [];
  const requiredItems = items.filter((i) => i.required);
  const postedCount = requiredItems.filter((i) => !!i.posted_at).length;
  const progress =
    requiredItems.length === 0
      ? 0
      : Math.round((postedCount / requiredItems.length) * 100);

  if (isLoading) {
    return (
      <Card padded>
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="skeleton h-10 w-full" />
          ))}
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card padded>
        <div className="flex items-start gap-3 text-critical-700">
          <AlertTriangle className="h-5 w-5 mt-0.5" />
          <div>
            <div className="font-medium">Couldn't load postings</div>
            <Button
              size="sm"
              variant="secondary"
              className="mt-3"
              onClick={() => refetch()}
            >
              Retry
            </Button>
          </div>
        </div>
      </Card>
    );
  }

  return (
    <>
      <Card padded className="mb-6">
        <div className="flex items-center justify-between gap-4 mb-2">
          <div>
            <h3 className="text-base font-semibold text-ink-900">
              Wall postings progress
            </h3>
            <p className="text-sm text-ink-700">
              {postedCount} of {requiredItems.length} required items posted.
            </p>
          </div>
          {data?.all_required_posted ? (
            <Badge variant="compliant">Complete</Badge>
          ) : (
            <Badge variant="warning">In progress</Badge>
          )}
        </div>
        <div className="h-2 w-full rounded-full bg-surface-sunken overflow-hidden">
          <div
            className={`h-full transition-all ${
              progress === 100 ? 'bg-brand-600' : 'bg-caution-500'
            }`}
            style={{ width: `${progress}%` }}
          />
        </div>
      </Card>

      <Card padded={false}>
        <ul className="divide-y divide-line-soft">
          {items.map((item) => (
            <PostingRow
              key={item.key}
              item={item}
              onToggle={(next) =>
                upsert.mutate({
                  key: item.key,
                  input: next
                    ? { posted_at: new Date().toISOString() }
                    : { unpost: true },
                })
              }
              pending={upsert.isPending && upsert.variables?.key === item.key}
            />
          ))}
        </ul>
      </Card>
    </>
  );
}

function PostingRow({
  item,
  onToggle,
  pending,
}: {
  item: PostingItem;
  onToggle: (next: boolean) => void;
  pending: boolean;
}): JSX.Element {
  const posted = !!item.posted_at;
  const [showPhotos, setShowPhotos] = useState(false);
  return (
    <li className="px-5 py-4">
      <div className="flex items-start gap-3">
        <button
          type="button"
          onClick={() => onToggle(!posted)}
          disabled={pending}
          aria-pressed={posted}
          aria-label={posted ? 'Mark as not posted' : 'Mark as posted'}
          className={`mt-0.5 h-6 w-6 flex-shrink-0 rounded-full border-2 grid place-items-center transition-colors ${
            posted
              ? 'bg-brand-600 border-brand-600 text-white'
              : 'border-line hover:border-brand-400'
          }`}
        >
          {posted ? (
            <CheckCircle2 className="h-4 w-4" />
          ) : (
            <Circle className="h-4 w-4 text-transparent" />
          )}
        </button>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-medium text-ink-900">{item.label}</span>
            {!item.required && <Badge variant="neutral">Optional</Badge>}
          </div>
          <div className="text-xs text-ink-500 mt-0.5">
            {posted ? `Posted ${formatDate(item.posted_at!)}` : 'Not yet posted'}
          </div>
        </div>
        <button
          type="button"
          onClick={() => setShowPhotos((v) => !v)}
          className="text-xs text-ink-600 hover:text-brand-700 self-center"
        >
          {showPhotos ? 'Hide photos' : 'Photos'}
        </button>
      </div>
      {showPhotos && (
        <div className="mt-3 ml-9 pt-3 border-t border-line-soft">
          <EvidencePhotos
            subjectKind="posting"
            subjectId={item.key}
            title={`Photos for "${item.label}"`}
            compact
          />
        </div>
      )}
    </li>
  );
}

// ---------------------------------------------------------------------------
// Ratio tab — REQ-039
// Ratios are prescribed by Florida (Handbook §3.4), not by an operator
// pref. Show today's room-by-room snapshot, computed from rooms +
// attendance, with calm color bands. No text input — there's nothing for
// the operator to type.
// ---------------------------------------------------------------------------

function RatioTab(): JSX.Element {
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['ratio-current'],
    queryFn: () => operationsApi.getCurrentRatio(),
    refetchInterval: 60_000, // recompute every minute so the room cards stay live
  });

  if (isLoading) {
    return (
      <Card padded>
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="skeleton h-16 w-full" />
          ))}
        </div>
      </Card>
    );
  }

  if (isError || !data) {
    return (
      <Card padded>
        <div className="flex items-start gap-3 text-critical-700">
          <AlertTriangle className="h-5 w-5 mt-0.5" />
          <div>
            <div className="font-medium">Couldn't load today's ratio</div>
            <Button
              size="sm"
              variant="secondary"
              className="mt-3"
              onClick={() => refetch()}
            >
              Retry
            </Button>
          </div>
        </div>
      </Card>
    );
  }

  return (
    <>
      <Card padded className="mb-6">
        <h3 className="text-base font-semibold text-ink-900">
          Today's ratio, room by room
        </h3>
        <p className="text-sm text-ink-700 mt-1">
          We compare today's attendance to Florida's required ratios. Tap a
          room to see how many more staff you need — or stop adding kids.
        </p>
        <div className="mt-3 flex flex-wrap gap-2 text-xs text-ink-700">
          <Badge variant="neutral">
            {data.staff_present_total} staff clocked in
          </Badge>
          <Badge variant="neutral">
            {data.children_present_total} children signed in
          </Badge>
          {data.any_out_of_ratio ? (
            <Badge variant="critical">One or more rooms over ratio</Badge>
          ) : (
            <Badge variant="compliant">All rooms within ratio</Badge>
          )}
        </div>
      </Card>

      {data.rooms.length === 0 ? (
        <EmptyState
          icon={<Users className="h-5 w-5 text-ink-500" />}
          title="No classrooms yet"
          description="Add a classroom on Settings → Facility so we can show you today's required ratios."
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {data.rooms.map((room) => (
            <RatioRoomCard key={room.room_id} room={room} />
          ))}
        </div>
      )}
    </>
  );
}

function RatioRoomCard({ room }: { room: RoomRatioStatus }): JSX.Element {
  const palette =
    room.status === 'out_of_ratio'
      ? 'border-critical-200 bg-critical-50'
      : room.status === 'tight'
        ? 'border-caution-200 bg-caution-50'
        : room.status === 'unknown'
          ? 'border-line-soft bg-white'
          : 'border-brand-200 bg-brand-50';

  const badge =
    room.status === 'out_of_ratio'
      ? { variant: 'critical' as const, text: 'Over ratio' }
      : room.status === 'tight'
        ? { variant: 'warning' as const, text: 'Tight' }
        : room.status === 'unknown'
          ? { variant: 'neutral' as const, text: 'Set age band' }
          : { variant: 'compliant' as const, text: 'Compliant' };

  return (
    <div className={`rounded-lg border p-4 ${palette}`}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h4 className="font-semibold text-ink-900">{room.room_name}</h4>
          <p className="text-xs text-ink-700 mt-0.5">{room.age_band_label}</p>
        </div>
        <Badge variant={badge.variant}>{badge.text}</Badge>
      </div>
      <dl className="mt-3 grid grid-cols-3 gap-3 text-sm">
        <div>
          <dt className="text-xs uppercase tracking-wide text-ink-500">
            Required ratio
          </dt>
          <dd className="font-medium text-ink-900">
            {room.mandated_ratio > 0 ? `1:${room.mandated_ratio}` : '—'}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-ink-500">
            Children here
          </dt>
          <dd className="font-medium text-ink-900">{room.children_present}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-ink-500">
            Staff present
          </dt>
          <dd className="font-medium text-ink-900">{room.staff_present}</dd>
        </div>
      </dl>
      {room.explainer && (
        <p className="mt-3 text-sm text-ink-700 leading-snug">
          {room.explainer}
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

function daysSince(iso: string): number {
  const ms = Date.now() - new Date(iso).getTime();
  return Math.max(0, Math.floor(ms / 86_400_000));
}

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return s === 0 ? `${m}m` : `${m}m ${s}s`;
}
