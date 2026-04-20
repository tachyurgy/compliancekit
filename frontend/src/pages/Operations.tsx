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
import {
  operationsApi,
  type DrillKind,
  type DrillLog,
  type PostingItem,
  type RoomInput,
  type RatioCheckResponse,
  DRILL_CADENCE_DAYS,
  drillLabel,
} from '@/api/operations';
import { formatDate, formatRelativeDays } from '@/lib/format';

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

      <div className="mb-6 flex flex-wrap gap-2 border-b border-gray-100">
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
                  : 'border-transparent text-gray-600 hover:text-gray-900'
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
  const lastByKind = useMemo(() => {
    const m = new Map<DrillKind, DrillLog>();
    for (const d of drills) {
      const prev = m.get(d.drill_kind);
      if (!prev || new Date(d.drill_date) > new Date(prev.drill_date)) {
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
                    : 'border-gray-100 bg-white'
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
                <div className="mt-1 text-xs text-gray-600">
                  {last
                    ? `Last run ${formatRelativeDays(last.drill_date)} (${formatDate(last.drill_date)})`
                    : 'Never logged'}
                </div>
                <div className="mt-0.5 text-xs text-gray-500">
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
          icon={<Flame className="h-5 w-5 text-gray-500" />}
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
                <tr className="text-left border-b border-gray-100 text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-5 py-3 font-medium">Kind</th>
                  <th className="px-5 py-3 font-medium">Date</th>
                  <th className="px-5 py-3 font-medium">Duration</th>
                  <th className="px-5 py-3 font-medium">Notes</th>
                  <th className="px-5 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {drills.map((d) => (
                  <tr key={d.id} className="hover:bg-surface-muted">
                    <td className="px-5 py-3 font-medium text-gray-900 capitalize">
                      {drillLabel(d.drill_kind)}
                    </td>
                    <td className="px-5 py-3 text-gray-600">
                      {formatDate(d.drill_date)}
                    </td>
                    <td className="px-5 py-3 text-gray-600">
                      {d.duration_seconds
                        ? formatDuration(d.duration_seconds)
                        : '—'}
                    </td>
                    <td className="px-5 py-3 text-gray-600 max-w-md truncate">
                      {d.notes || '—'}
                    </td>
                    <td className="px-5 py-3 text-right">
                      <button
                        type="button"
                        aria-label="Delete drill"
                        onClick={() => {
                          if (confirm('Delete this drill log?')) {
                            deleteMutation.mutate(d.id);
                          }
                        }}
                        className="p-1 rounded text-gray-400 hover:text-critical-600 hover:bg-critical-50"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </td>
                  </tr>
                ))}
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
      description="Record what was practiced, when, and how long it took. Staff signatures can be attached later."
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
          <label className="block text-sm font-medium text-gray-800 mb-1">
            Notes
          </label>
          <textarea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            rows={3}
            className="block w-full rounded-lg border border-gray-200 bg-white text-sm px-3 py-2 focus:outline-none focus:ring-2 focus:ring-brand-600 focus:ring-opacity-20 focus:border-brand-600"
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
            <h3 className="text-base font-semibold text-gray-900">
              Wall postings progress
            </h3>
            <p className="text-sm text-gray-600">
              {postedCount} of {requiredItems.length} required items posted.
            </p>
          </div>
          {data?.all_required_posted ? (
            <Badge variant="compliant">Complete</Badge>
          ) : (
            <Badge variant="warning">In progress</Badge>
          )}
        </div>
        <div className="h-2 w-full rounded-full bg-gray-100 overflow-hidden">
          <div
            className={`h-full transition-all ${
              progress === 100 ? 'bg-brand-600' : 'bg-caution-500'
            }`}
            style={{ width: `${progress}%` }}
          />
        </div>
      </Card>

      <Card padded={false}>
        <ul className="divide-y divide-gray-100">
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
  return (
    <li className="flex items-start gap-3 px-5 py-4">
      <button
        type="button"
        onClick={() => onToggle(!posted)}
        disabled={pending}
        aria-pressed={posted}
        aria-label={posted ? 'Mark as not posted' : 'Mark as posted'}
        className={`mt-0.5 h-6 w-6 flex-shrink-0 rounded-full border-2 grid place-items-center transition-colors ${
          posted
            ? 'bg-brand-600 border-brand-600 text-white'
            : 'border-gray-300 hover:border-brand-400'
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
          <span className="font-medium text-gray-900">{item.label}</span>
          {!item.required && <Badge variant="neutral">Optional</Badge>}
        </div>
        <div className="text-xs text-gray-500 mt-0.5">
          {posted
            ? `Posted ${formatDate(item.posted_at!)}`
            : 'Not yet posted'}
          {item.photo_document_id && ' • photo on file'}
        </div>
      </div>
      {/* Photo upload is stubbed — the Documents upload flow already exists; hook it up later. */}
    </li>
  );
}

// ---------------------------------------------------------------------------
// Ratio tab
// ---------------------------------------------------------------------------

interface RoomRow extends RoomInput {
  _id: string;
}

function newRoom(): RoomRow {
  return {
    _id: Math.random().toString(36).slice(2),
    label: '',
    age_months_low: 24,
    age_months_high: 36,
    children_present: 0,
    staff_present: 1,
  };
}

function RatioTab(): JSX.Element {
  const qc = useQueryClient();
  const [rooms, setRooms] = useState<RoomRow[]>(() => [
    { ...newRoom(), label: 'Room 1' },
  ]);
  const [result, setResult] = useState<RatioCheckResponse | null>(null);

  const check = useMutation({
    mutationFn: () =>
      operationsApi.checkRatio(
        rooms.map(({ _id, ...r }) => ({ ...r, label: r.label || 'Room' })),
      ),
    onSuccess: (res) => {
      setResult(res);
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
      toast[res.ok ? 'success' : 'warning'](
        res.ok
          ? 'All rooms are within ratio.'
          : `Over ratio in: ${res.violated_rooms.join(', ')}`,
      );
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Ratio check failed');
    },
  });

  function update(id: string, patch: Partial<RoomInput>) {
    setRooms((prev) =>
      prev.map((r) => (r._id === id ? { ...r, ...patch } : r)),
    );
  }

  function removeRoom(id: string) {
    setRooms((prev) => prev.filter((r) => r._id !== id));
  }

  const resultByLabel = useMemo(() => {
    const m = new Map<string, RatioCheckResponse['rooms'][number]>();
    if (result) {
      for (const r of result.rooms) {
        m.set(r.label, r);
      }
    }
    return m;
  }, [result]);

  return (
    <>
      <Card title="Rooms" padded className="mb-6">
        <div className="space-y-4">
          {rooms.map((r) => {
            const rr = resultByLabel.get(r.label || 'Room');
            return (
              <div
                key={r._id}
                className={`rounded-lg border p-4 ${
                  rr
                    ? rr.in_ratio
                      ? 'border-brand-200 bg-brand-50'
                      : 'border-critical-200 bg-critical-50'
                    : 'border-gray-100 bg-white'
                }`}
              >
                <div className="grid grid-cols-1 md:grid-cols-5 gap-3">
                  <Input
                    label="Room label"
                    value={r.label}
                    onChange={(e) => update(r._id, { label: e.target.value })}
                    placeholder="e.g. Infants"
                  />
                  <Input
                    label="Min age (mo)"
                    type="number"
                    min={0}
                    value={r.age_months_low}
                    onChange={(e) =>
                      update(r._id, {
                        age_months_low: parseInt(e.target.value || '0', 10),
                      })
                    }
                  />
                  <Input
                    label="Max age (mo)"
                    type="number"
                    min={0}
                    value={r.age_months_high}
                    onChange={(e) =>
                      update(r._id, {
                        age_months_high: parseInt(e.target.value || '0', 10),
                      })
                    }
                  />
                  <Input
                    label="Children present"
                    type="number"
                    min={0}
                    value={r.children_present}
                    onChange={(e) =>
                      update(r._id, {
                        children_present: parseInt(e.target.value || '0', 10),
                      })
                    }
                  />
                  <Input
                    label="Staff present"
                    type="number"
                    min={0}
                    value={r.staff_present}
                    onChange={(e) =>
                      update(r._id, {
                        staff_present: parseInt(e.target.value || '0', 10),
                      })
                    }
                  />
                </div>
                <div className="mt-3 flex items-center justify-between gap-3">
                  <div className="text-xs text-gray-600">
                    {rr ? (
                      rr.in_ratio ? (
                        <Badge variant="compliant">
                          In ratio — cap 1:{rr.ratio_cap}
                        </Badge>
                      ) : (
                        <Badge variant="critical">
                          OVER ratio — cap 1:{rr.ratio_cap}, actual 1:
                          {rr.actual_ratio.toFixed(1)}
                        </Badge>
                      )
                    ) : (
                      <span>Enter values and run the check.</span>
                    )}
                  </div>
                  {rooms.length > 1 && (
                    <button
                      type="button"
                      onClick={() => removeRoom(r._id)}
                      className="text-sm text-gray-500 hover:text-critical-600 inline-flex items-center gap-1"
                    >
                      <Trash2 className="h-4 w-4" /> Remove
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        <div className="mt-4 flex flex-wrap justify-between gap-3">
          <Button
            variant="secondary"
            leftIcon={<Plus className="h-4 w-4" />}
            onClick={() =>
              setRooms((prev) => [
                ...prev,
                { ...newRoom(), label: `Room ${prev.length + 1}` },
              ])
            }
          >
            Add room
          </Button>
          <Button
            loading={check.isPending}
            onClick={() => check.mutate()}
            disabled={rooms.length === 0}
          >
            Check ratio
          </Button>
        </div>
      </Card>

      {result && (
        <Card
          title={result.ok ? 'All rooms in ratio' : 'Over ratio'}
          description={
            result.ok
              ? 'Staffing meets or beats the state cap for every room.'
              : `Rooms over the cap: ${result.violated_rooms.join(', ')}`
          }
          padded
        >
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-gray-500">
                <th className="py-2 font-medium">Room</th>
                <th className="py-2 font-medium">Cap</th>
                <th className="py-2 font-medium">Actual</th>
                <th className="py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {result.rooms.map((r, i) => (
                <tr key={`${r.label}-${i}`}>
                  <td className="py-2">{r.label}</td>
                  <td className="py-2">1:{r.ratio_cap || '—'}</td>
                  <td className="py-2">
                    {r.actual_ratio > 0 ? `1:${r.actual_ratio.toFixed(1)}` : '—'}
                  </td>
                  <td className="py-2">
                    <Badge variant={r.in_ratio ? 'compliant' : 'critical'}>
                      {r.in_ratio ? 'In ratio' : 'Over'}
                    </Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </>
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
