import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowLeft,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  ScrollText,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import Select from '@/components/common/Select';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import {
  auditLogApi,
  AUDIT_LOG_ACTIONS,
  AUDIT_LOG_TARGET_KINDS,
  type AuditLogItem,
} from '@/api/auditLog';
import { formatTimestamp } from '@/lib/format';

// The backend clamps limit to [1,500]; 20 is the product-spec page size.
const PAGE_SIZE = 20;

interface Filters {
  action: string;
  target_kind: string;
  since: string;
  until: string;
}

const EMPTY_FILTERS: Filters = {
  action: '',
  target_kind: '',
  since: '',
  until: '',
};

export default function SettingsAuditLog(): JSX.Element {
  const [filters, setFilters] = useState<Filters>(EMPTY_FILTERS);
  const [pageCount, setPageCount] = useState(1);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const effectiveLimit = PAGE_SIZE * pageCount;

  const {
    data,
    isLoading,
    isError,
    refetch,
  } = useQuery({
    queryKey: ['audit-log', filters, effectiveLimit],
    queryFn: () =>
      auditLogApi.list({
        limit: effectiveLimit,
        action: filters.action || undefined,
        target_kind: filters.target_kind || undefined,
        since: filters.since ? toIsoOrUndefined(filters.since) : undefined,
        until: filters.until ? toIsoOrUndefined(filters.until, true) : undefined,
      }),
  });

  const items = data?.items ?? [];
  const hasMore = Boolean(data?.next_cursor);

  const actionOptions = useMemo(
    () => [
      { value: '', label: 'All actions' },
      ...AUDIT_LOG_ACTIONS.map((a) => ({ value: a, label: a })),
    ],
    [],
  );
  const targetOptions = useMemo(
    () => [
      { value: '', label: 'All target kinds' },
      ...AUDIT_LOG_TARGET_KINDS.map((t) => ({ value: t, label: t })),
    ],
    [],
  );

  const toggleExpanded = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const updateFilter = (k: keyof Filters, v: string) => {
    setFilters((prev) => ({ ...prev, [k]: v }));
    setPageCount(1); // a filter change always resets pagination
    setExpanded(new Set());
  };

  const resetAll = () => {
    setFilters(EMPTY_FILTERS);
    setPageCount(1);
    setExpanded(new Set());
  };

  return (
    <div>
      <Link
        to="/settings"
        className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 mb-4"
      >
        <ArrowLeft className="h-4 w-4" /> Back to settings
      </Link>
      <PageHeader
        title="Audit log"
        description="Every change to your facility data is recorded here. Retained for 7 years."
        actions={
          <Button size="sm" variant="ghost" onClick={resetAll}>
            Clear filters
          </Button>
        }
      />

      <Card padded className="mb-4">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <Select
            label="Action"
            value={filters.action}
            onChange={(e) => updateFilter('action', e.target.value)}
            options={actionOptions}
          />
          <Select
            label="Target kind"
            value={filters.target_kind}
            onChange={(e) => updateFilter('target_kind', e.target.value)}
            options={targetOptions}
          />
          <Input
            label="Since"
            type="date"
            value={filters.since}
            onChange={(e) => updateFilter('since', e.target.value)}
          />
          <Input
            label="Until"
            type="date"
            value={filters.until}
            onChange={(e) => updateFilter('until', e.target.value)}
          />
        </div>
      </Card>

      {isLoading ? (
        <LoadingSpinner />
      ) : isError ? (
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load audit log</div>
              <Button
                size="sm"
                variant="secondary"
                className="mt-2"
                onClick={() => refetch()}
              >
                Retry
              </Button>
            </div>
          </div>
        </Card>
      ) : items.length === 0 ? (
        <Card padded>
          <div className="flex items-center gap-3 text-gray-600">
            <ScrollText className="h-5 w-5" />
            <span>No audit events match these filters.</span>
          </div>
        </Card>
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-4 py-3 w-8"></th>
                  <th className="px-4 py-3">Timestamp</th>
                  <th className="px-4 py-3">Actor</th>
                  <th className="px-4 py-3">Action</th>
                  <th className="px-4 py-3">Target</th>
                  <th className="px-4 py-3">IP</th>
                </tr>
              </thead>
              <tbody>
                {items.map((row) => {
                  const isOpen = expanded.has(row.id);
                  return (
                    <AuditRow
                      key={row.id}
                      item={row}
                      open={isOpen}
                      onToggle={() => toggleExpanded(row.id)}
                    />
                  );
                })}
              </tbody>
            </table>
          </div>
          {hasMore && (
            <div className="p-4 border-t border-gray-100 flex justify-center">
              <Button
                size="sm"
                variant="secondary"
                onClick={() => setPageCount((n) => n + 1)}
              >
                Load more
              </Button>
            </div>
          )}
        </Card>
      )}
    </div>
  );
}

function AuditRow({
  item,
  open,
  onToggle,
}: {
  item: AuditLogItem;
  open: boolean;
  onToggle: () => void;
}): JSX.Element {
  const actorLabel = actorDisplay(item);
  const targetLabel = targetDisplay(item);
  return (
    <>
      <tr
        className="border-t border-gray-100 hover:bg-gray-50 cursor-pointer"
        onClick={onToggle}
      >
        <td className="px-4 py-3 text-gray-400">
          {open ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </td>
        <td className="px-4 py-3 whitespace-nowrap text-gray-700">
          {formatTimestamp(item.created_at)}
        </td>
        <td className="px-4 py-3 text-gray-900">{actorLabel}</td>
        <td className="px-4 py-3">
          <code className="text-xs bg-gray-100 px-2 py-0.5 rounded">{item.action}</code>
        </td>
        <td className="px-4 py-3 text-gray-700">{targetLabel}</td>
        <td className="px-4 py-3 text-gray-500 text-xs font-mono">{item.ip || '—'}</td>
      </tr>
      {open && (
        <tr className="bg-gray-50 border-t border-gray-100">
          <td></td>
          <td colSpan={5} className="px-4 py-3">
            <dl className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
              <DetailRow label="Event ID" value={item.id} mono />
              <DetailRow label="Actor kind" value={item.actor_kind} />
              {item.actor_id && <DetailRow label="Actor ID" value={item.actor_id} mono />}
              {item.actor_email && <DetailRow label="Actor email" value={item.actor_email} />}
              {item.target_kind && (
                <DetailRow label="Target kind" value={item.target_kind} />
              )}
              {item.target_id && <DetailRow label="Target ID" value={item.target_id} mono />}
              {item.user_agent && (
                <DetailRow label="User agent" value={item.user_agent} />
              )}
            </dl>
            <div className="mt-3">
              <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">
                Metadata
              </div>
              <pre className="text-xs bg-white border border-gray-200 rounded p-3 overflow-x-auto whitespace-pre-wrap break-words">
                {JSON.stringify(item.metadata ?? {}, null, 2)}
              </pre>
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

function DetailRow({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}): JSX.Element {
  return (
    <div className="flex gap-2">
      <dt className="text-gray-500 min-w-[90px]">{label}:</dt>
      <dd className={`text-gray-900 ${mono ? 'font-mono break-all' : ''}`}>{value}</dd>
    </div>
  );
}

function actorDisplay(item: AuditLogItem): string {
  if (item.actor_email) return item.actor_email;
  switch (item.actor_kind) {
    case 'system':
      return 'system';
    case 'webhook':
      return 'webhook';
    case 'parent':
      return 'parent (portal)';
    case 'staff':
      return 'staff (portal)';
    case 'provider_admin':
      return item.actor_id ? `admin (${item.actor_id.slice(0, 8)}…)` : 'admin';
    default:
      return item.actor_kind;
  }
}

function targetDisplay(item: AuditLogItem): string {
  if (!item.target_kind && !item.target_id) return '—';
  if (!item.target_id) return item.target_kind;
  return `${item.target_kind}: ${item.target_id}`;
}

/**
 * Convert a YYYY-MM-DD date from an <input type="date"> into an ISO timestamp.
 * When `endOfDay` is true we pin to 23:59:59 so the "until" filter includes
 * events that happened later in that day. Otherwise we pin to 00:00:00.
 */
function toIsoOrUndefined(yyyyMmDd: string, endOfDay = false): string | undefined {
  if (!yyyyMmDd) return undefined;
  const time = endOfDay ? 'T23:59:59Z' : 'T00:00:00Z';
  const candidate = new Date(yyyyMmDd + time);
  if (Number.isNaN(candidate.getTime())) return undefined;
  return candidate.toISOString();
}
