import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Mail, Phone, MessageSquare, Send, AlertCircle, Clock, Zap, MessageCircle } from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import EmptyState from '@/components/common/EmptyState';
import { toast } from '@/components/common/Toast';
import { chaseApi, type ChaseRow, type ChaseActivityItem } from '@/api/chase';
import { formatDate } from '@/lib/format';

// REQ-042: Chase Mode dashboard. Three sections, single page.
//
// Calm voice: this page makes the chase engine visible without leaning on
// red. Yellow + ink + brand-700; "needs a personal call" is the only band
// where critical-50 shows up.
export default function ChasePage(): JSX.Element {
  const qc = useQueryClient();
  const state = useQuery({
    queryKey: ['chase', 'state'],
    queryFn: () => chaseApi.state(),
  });
  const activity = useQuery({
    queryKey: ['chase', 'activity'],
    queryFn: () => chaseApi.activity(),
  });
  const trigger = useMutation({
    mutationFn: (row: ChaseRow) =>
      chaseApi.trigger({
        recipientKind: row.recipientKind,
        recipientId: row.recipientId,
        docKind: undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['chase'] });
    },
  });

  async function sendNudge(row: ChaseRow) {
    try {
      await trigger.mutateAsync(row);
      toast.success(`Nudge sent to ${row.recipientName}.`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "We couldn't send that nudge.");
    }
  }

  if (state.isLoading || activity.isLoading) {
    return (
      <div className="py-10">
        <LoadingSpinner label="Loading Chase Mode" />
      </div>
    );
  }
  if (state.isError) {
    return (
      <div className="py-10 text-center text-sm text-ink-700">
        We couldn't load Chase Mode. Try again in a moment.
      </div>
    );
  }

  const outstanding = state.data?.outstanding ?? [];
  const stuck = state.data?.stuck ?? [];
  const activityItems = activity.data ?? [];

  return (
    <div>
      <PageHeader
        title={
          <span className="inline-flex items-center gap-2">
            <span className="inline-flex items-center justify-center h-9 w-9 rounded-full bg-brand-50 text-brand-700">
              <Zap className="h-4 w-4" />
            </span>
            Chase Mode
          </span>
        }
        description="Levelbrook does the chasing. Here's what's open right now, what already went out, and who needs a personal call."
        actions={
          <Link to="/messages/new">
            <Button leftIcon={<MessageCircle className="h-4 w-4" />}>Send a message</Button>
          </Link>
        }
      />

      {stuck.length > 0 && (
        <Card
          title={`Needs a personal call (${stuck.length})`}
          description="These people got 3+ nudges with no answer. A quick call usually does it."
          className="mb-6 border-critical-200"
          padded={false}
        >
          <ul className="divide-y divide-line-soft">
            {stuck.map((row, idx) => (
              <li key={stuckKey(row, idx)} className="px-5 py-3 flex flex-wrap items-center gap-3 justify-between">
                <div className="min-w-0">
                  <RecipientLink row={row} />
                  <div className="text-xs text-ink-500 mt-0.5">
                    {row.itemLabel}
                    {row.dueOn && ` · due ${formatDate(row.dueOn)}`}
                    {' · '}
                    {row.nagCount} nudges in 60 days
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {row.recipientPhone && (
                    <a
                      href={`tel:${row.recipientPhone}`}
                      className="inline-flex items-center gap-1 text-sm font-medium text-brand-700"
                    >
                      <Phone className="h-3.5 w-3.5" /> Call
                    </a>
                  )}
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => sendNudge(row)}
                    leftIcon={<Send className="h-4 w-4" />}
                    loading={trigger.isPending && trigger.variables?.recipientId === row.recipientId}
                  >
                    Send a nudge
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card
        title={`What's outstanding (${outstanding.length})`}
        description="One row per person + missing item. Tap 'Send a nudge' to fire a reminder right now."
        padded={false}
        className="mb-6"
      >
        {outstanding.length === 0 ? (
          <div className="p-6">
            <EmptyState
              title="All caught up"
              description="Every required document is on file. Nothing to chase."
            />
          </div>
        ) : (
          <ul className="divide-y divide-line-soft">
            {outstanding.map((row, idx) => (
              <li key={stuckKey(row, idx)} className="px-5 py-3 flex flex-wrap items-center gap-3 justify-between">
                <div className="min-w-0 flex-1">
                  <RecipientLink row={row} />
                  <div className="text-xs text-ink-500 mt-0.5 flex flex-wrap items-center gap-x-2">
                    <span>{row.itemLabel}</span>
                    {row.dueOn && <span>· due {formatDate(row.dueOn)}</span>}
                    <span>·</span>
                    <span className="inline-flex items-center gap-1">
                      <ChannelIcon ch={row.channel} /> {channelLabel(row.channel)}
                    </span>
                    {row.lastNagAt ? (
                      <span>· last nudge {formatDate(row.lastNagAt)}</span>
                    ) : (
                      <span>· no nudges yet</span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <StatusBadge status={row.status} />
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => sendNudge(row)}
                    leftIcon={<Send className="h-4 w-4" />}
                    loading={trigger.isPending && trigger.variables?.recipientId === row.recipientId}
                  >
                    Send a nudge
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card title="Activity — last 30 days" padded={false}>
        {activityItems.length === 0 ? (
          <div className="p-6">
            <EmptyState
              title="Quiet so far"
              description="No nudges have gone out in the last 30 days."
            />
          </div>
        ) : (
          <ActivityFeed items={activityItems} />
        )}
      </Card>
    </div>
  );
}

function stuckKey(row: ChaseRow, idx: number): string {
  return `${row.recipientKind}:${row.recipientId}:${row.itemLabel}:${idx}`;
}

function RecipientLink({ row }: { row: ChaseRow }): JSX.Element {
  const to = row.recipientKind === 'staff' ? `/staff/${row.recipientId}` : `/children/${row.recipientId}`;
  const tag = row.recipientKind === 'staff' ? `Staff: ${row.recipientRole || 'team'}` : 'Parent';
  return (
    <Link to={to} className="text-sm font-medium text-ink-900 hover:text-brand-700">
      {row.recipientName}
      <span className="ml-2 text-xs font-normal text-ink-500">{tag}</span>
    </Link>
  );
}

function ChannelIcon({ ch }: { ch: string }): JSX.Element {
  if (ch === 'sms') return <MessageSquare className="h-3 w-3" />;
  if (ch === 'both') return <Mail className="h-3 w-3" />;
  return <Mail className="h-3 w-3" />;
}

function channelLabel(ch: string): string {
  if (ch === 'sms') return 'text only';
  if (ch === 'both') return 'email + text';
  return 'email';
}

function StatusBadge({ status }: { status: ChaseRow['status'] }): JSX.Element {
  const label =
    status === 'missing' ? 'Missing' : status === 'expired' ? 'Expired' : 'Expiring soon';
  const cls =
    status === 'missing'
      ? 'bg-caution-50 text-caution-700 ring-caution-200'
      : status === 'expired'
        ? 'bg-critical-50 text-critical-700 ring-critical-200'
        : 'bg-brand-50 text-brand-700 ring-brand-200';
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ring-1 ${cls}`}>
      {status === 'expired' ? <AlertCircle className="h-3 w-3" /> : <Clock className="h-3 w-3" />}
      {label}
    </span>
  );
}

function ActivityFeed({ items }: { items: ChaseActivityItem[] }): JSX.Element {
  // Group by ISO date (YYYY-MM-DD).
  const byDay = new Map<string, ChaseActivityItem[]>();
  for (const it of items) {
    const day = (it.createdAt || '').slice(0, 10) || 'unknown';
    const bucket = byDay.get(day) ?? [];
    bucket.push(it);
    byDay.set(day, bucket);
  }
  const days = Array.from(byDay.keys()).sort().reverse();

  return (
    <ul className="divide-y divide-line-soft">
      {days.map((day) => (
        <li key={day} className="px-5 py-3">
          <div className="text-xs uppercase tracking-wider text-ink-500 mb-2">
            {day === 'unknown' ? 'Unknown date' : formatDate(day)}
          </div>
          <ul className="space-y-1.5">
            {(byDay.get(day) ?? []).map((it) => (
              <li key={it.id} className="text-sm text-ink-700 flex items-center gap-2">
                <ActivityIcon ch={it.channel} status={describeStatus(it)} />
                <span>
                  {verbFor(it.channel)} <span className="font-medium text-ink-900">{it.recipientName}</span>{' '}
                  re: {it.itemLabel}
                </span>
                <span className="text-xs text-ink-500">{timeFor(it.createdAt)}</span>
              </li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  );
}

function describeStatus(it: ChaseActivityItem): 'ok' | 'failed' {
  if ((it.emailStatus || '').startsWith('failed') || (it.smsStatus || '').startsWith('failed')) return 'failed';
  return 'ok';
}

function ActivityIcon({ ch, status }: { ch: string; status: 'ok' | 'failed' }): JSX.Element {
  const Icon = ch === 'sms' || ch === 'both' ? MessageSquare : Mail;
  const tone = status === 'failed' ? 'text-critical-600' : 'text-brand-700';
  return <Icon className={`h-3.5 w-3.5 ${tone}`} />;
}

function verbFor(ch: string): string {
  if (ch === 'sms') return 'Texted';
  if (ch === 'both') return 'Emailed + texted';
  return 'Emailed';
}

function timeFor(iso: string): string {
  // Backend stores SQLite timestamps as "YYYY-MM-DD HH:MM:SS".
  if (!iso) return '';
  const time = iso.slice(11, 16);
  return time ? `at ${time}` : '';
}
