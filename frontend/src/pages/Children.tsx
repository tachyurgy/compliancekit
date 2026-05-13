import { Link } from 'react-router-dom';
import { Baby, Plus, AlertTriangle, Search } from 'lucide-react';
import { useMemo, useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Button from '@/components/common/Button';
import Badge from '@/components/common/Badge';
import Input from '@/components/common/Input';
import Card from '@/components/common/Card';
import EmptyState from '@/components/common/EmptyState';
import AddChildModal from '@/components/AddChildModal';
import { useChildren } from '@/hooks/useChildren';
import { formatDate, fullName } from '@/lib/format';

function statusVariant(status: 'compliant' | 'warning' | 'critical') {
  return status === 'compliant' ? 'compliant' : status === 'warning' ? 'warning' : 'critical';
}

const IMMUNIZATION_LABEL: Record<string, string> = {
  up_to_date: 'Up to date',
  due_soon: 'Due soon',
  overdue: 'Overdue',
};

export default function Children(): JSX.Element {
  const { data: children = [], isLoading, isError, error, refetch } = useChildren();
  const [query, setQuery] = useState('');
  const [addOpen, setAddOpen] = useState(false);

  const filtered = useMemo(() => {
    if (!query.trim()) return children;
    const q = query.toLowerCase();
    return children.filter(
      (c) =>
        fullName(c.firstName, c.lastName).toLowerCase().includes(q) ||
        (c.parentName ?? '').toLowerCase().includes(q),
    );
  }, [children, query]);

  return (
    <div>
      <PageHeader
        eyebrow="Enrollment files"
        title="Children"
        description="Your roster. Tap any row to see required documents and immunization status. We'll chase the parents for what's missing."
        actions={
          <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setAddOpen(true)}>
            Add a child
          </Button>
        }
      />
      <AddChildModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={() => void refetch()}
      />

      <div className="mb-5 max-w-sm">
        <Input
          placeholder="Search by name or parent"
          leftIcon={<Search className="h-4 w-4" />}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {isLoading ? (
        <Card padded>
          <div className="space-y-2.5">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="skeleton h-12 w-full" />
            ))}
          </div>
        </Card>
      ) : isError ? (
        <div
          className="rounded-lb border border-critical-200 bg-critical-50/70 p-5 flex items-start gap-3"
          role="alert"
        >
          <AlertTriangle className="h-5 w-5 mt-0.5 text-critical-600 shrink-0" />
          <div className="flex-1">
            <div className="font-semibold text-critical-700">
              We couldn't load your roster just now
            </div>
            <div className="text-sm text-ink-700 mt-1 leading-relaxed">
              {error instanceof Error
                ? error.message
                : 'Check your internet and try again in a moment.'}
            </div>
            <Button
              className="mt-3"
              size="sm"
              variant="secondary"
              onClick={() => refetch()}
            >
              Try again
            </Button>
          </div>
        </div>
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<Baby className="h-6 w-6" />}
          title={query ? 'No matches' : 'No children enrolled yet'}
          description={
            query
              ? 'No children match that search. Try a different one.'
              : "Add your first child to get started. We'll chase the parents for any missing forms."
          }
          action={
            !query && (
              <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setAddOpen(true)}>
                Add a child
              </Button>
            )
          }
        />
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-line-soft text-xs uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-6 py-3.5 font-semibold">Name</th>
                  <th className="px-6 py-3.5 font-semibold">Date of birth</th>
                  <th className="px-6 py-3.5 font-semibold">Documents</th>
                  <th className="px-6 py-3.5 font-semibold">Immunizations</th>
                  <th className="px-6 py-3.5 font-semibold">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {filtered.map((c) => (
                  <tr key={c.id} className="hover:bg-surface-sunken/50 transition-colors">
                    <td className="px-6 py-3.5">
                      <Link
                        to={`/children/${c.id}`}
                        className="font-medium text-ink-900 hover:text-brand-700"
                      >
                        {fullName(c.firstName, c.lastName)}
                      </Link>
                      {c.parentName && (
                        <div className="text-xs text-ink-500 mt-0.5">
                          Parent: {c.parentName}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-3.5 text-ink-700">{formatDate(c.dateOfBirth)}</td>
                    <td className="px-6 py-3.5 text-ink-700">
                      <span className="font-medium text-ink-900">
                        {c.completedDocsCount}
                      </span>
                      <span className="text-ink-400"> / {c.requiredDocsCount}</span>
                    </td>
                    <td className="px-6 py-3.5">
                      <Badge
                        variant={
                          c.immunizationStatus === 'up_to_date'
                            ? 'compliant'
                            : c.immunizationStatus === 'due_soon'
                              ? 'warning'
                              : c.immunizationStatus === 'overdue'
                                ? 'critical'
                                : 'neutral'
                        }
                      >
                        {IMMUNIZATION_LABEL[c.immunizationStatus] ??
                          c.immunizationStatus.replace(/_/g, ' ')}
                      </Badge>
                    </td>
                    <td className="px-6 py-3.5">
                      <Badge variant={statusVariant(c.complianceStatus)}>
                        {c.complianceStatus === 'compliant'
                          ? 'On file'
                          : c.complianceStatus === 'warning'
                            ? 'Needs attention'
                            : "Let's fix this"}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}
