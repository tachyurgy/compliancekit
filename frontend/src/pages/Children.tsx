import { Link } from 'react-router-dom';
import { Baby, Plus, AlertTriangle, Search } from 'lucide-react';
import { useMemo, useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Button from '@/components/common/Button';
import Badge from '@/components/common/Badge';
import Input from '@/components/common/Input';
import Card from '@/components/common/Card';
import EmptyState from '@/components/common/EmptyState';
import { useChildren } from '@/hooks/useChildren';
import { formatDate, fullName } from '@/lib/format';

function statusVariant(status: 'compliant' | 'warning' | 'critical') {
  return status === 'compliant' ? 'compliant' : status === 'warning' ? 'warning' : 'critical';
}

export default function Children(): JSX.Element {
  const { data: children = [], isLoading, isError, error, refetch } = useChildren();
  const [query, setQuery] = useState('');

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
        title="Children"
        description="Roster of enrolled children. Click any row to view required documents and immunization status."
        actions={
          <Button leftIcon={<Plus className="h-4 w-4" />}>Add child</Button>
        }
      />

      <div className="mb-4 max-w-sm">
        <Input
          placeholder="Search by name"
          leftIcon={<Search className="h-4 w-4" />}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

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
              <div className="font-medium">Couldn't load children</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Try again shortly.'}
              </div>
              <Button
                className="mt-3"
                size="sm"
                variant="secondary"
                onClick={() => refetch()}
              >
                Retry
              </Button>
            </div>
          </div>
        </Card>
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<Baby className="h-5 w-5 text-gray-500" />}
          title={query ? 'No matches' : 'No children enrolled yet'}
          description={
            query
              ? 'Try a different search.'
              : 'Add your first child to start tracking required documents.'
          }
          action={
            !query && <Button leftIcon={<Plus className="h-4 w-4" />}>Add child</Button>
          }
        />
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-gray-100 text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-5 py-3 font-medium">Name</th>
                  <th className="px-5 py-3 font-medium">DOB</th>
                  <th className="px-5 py-3 font-medium">Docs</th>
                  <th className="px-5 py-3 font-medium">Immunizations</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {filtered.map((c) => (
                  <tr key={c.id} className="hover:bg-surface-muted">
                    <td className="px-5 py-3">
                      <Link
                        to={`/children/${c.id}`}
                        className="font-medium text-gray-900 hover:text-brand-700"
                      >
                        {fullName(c.firstName, c.lastName)}
                      </Link>
                      {c.parentName && (
                        <div className="text-xs text-gray-500">Parent: {c.parentName}</div>
                      )}
                    </td>
                    <td className="px-5 py-3 text-gray-600">{formatDate(c.dateOfBirth)}</td>
                    <td className="px-5 py-3 text-gray-600">
                      {c.completedDocsCount}/{c.requiredDocsCount}
                    </td>
                    <td className="px-5 py-3">
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
                        {c.immunizationStatus.replace(/_/g, ' ')}
                      </Badge>
                    </td>
                    <td className="px-5 py-3">
                      <Badge variant={statusVariant(c.complianceStatus)}>
                        {c.complianceStatus}
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
