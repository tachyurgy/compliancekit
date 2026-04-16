import { Link } from 'react-router-dom';
import { Users, Plus, AlertTriangle, Search } from 'lucide-react';
import { useMemo, useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Button from '@/components/common/Button';
import Badge from '@/components/common/Badge';
import Input from '@/components/common/Input';
import Card from '@/components/common/Card';
import EmptyState from '@/components/common/EmptyState';
import { useStaffList } from '@/hooks/useStaff';
import { fullName } from '@/lib/format';

const ROLE_LABEL: Record<string, string> = {
  director: 'Director',
  lead_teacher: 'Lead Teacher',
  assistant: 'Assistant',
  aide: 'Aide',
  cook: 'Cook',
  other: 'Other',
};

export default function Staff(): JSX.Element {
  const { data: staff = [], isLoading, isError, error, refetch } = useStaffList();
  const [query, setQuery] = useState('');

  const filtered = useMemo(() => {
    if (!query.trim()) return staff;
    const q = query.toLowerCase();
    return staff.filter((s) =>
      fullName(s.firstName, s.lastName).toLowerCase().includes(q),
    );
  }, [staff, query]);

  return (
    <div>
      <PageHeader
        title="Staff"
        description="Your team roster. Click any staff member to view their certifications and training hours."
        actions={<Button leftIcon={<Plus className="h-4 w-4" />}>Add staff</Button>}
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
              <div className="font-medium">Couldn't load staff</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Please try again.'}
              </div>
              <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
                Retry
              </Button>
            </div>
          </div>
        </Card>
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<Users className="h-5 w-5 text-gray-500" />}
          title={query ? 'No matches' : 'No staff yet'}
          description={query ? 'Try a different search.' : 'Add staff to begin tracking certifications.'}
          action={!query && <Button leftIcon={<Plus className="h-4 w-4" />}>Add staff</Button>}
        />
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-gray-100 text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-5 py-3 font-medium">Name</th>
                  <th className="px-5 py-3 font-medium">Role</th>
                  <th className="px-5 py-3 font-medium">Background check</th>
                  <th className="px-5 py-3 font-medium">Training</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {filtered.map((s) => (
                  <tr key={s.id} className="hover:bg-surface-muted">
                    <td className="px-5 py-3">
                      <Link
                        to={`/staff/${s.id}`}
                        className="font-medium text-gray-900 hover:text-brand-700"
                      >
                        {fullName(s.firstName, s.lastName)}
                      </Link>
                      {s.email && (
                        <div className="text-xs text-gray-500">{s.email}</div>
                      )}
                    </td>
                    <td className="px-5 py-3 text-gray-600">{ROLE_LABEL[s.role] ?? s.role}</td>
                    <td className="px-5 py-3">
                      <Badge
                        variant={
                          s.backgroundCheckStatus === 'cleared'
                            ? 'compliant'
                            : s.backgroundCheckStatus === 'pending'
                              ? 'warning'
                              : 'critical'
                        }
                      >
                        {s.backgroundCheckStatus.replace(/_/g, ' ')}
                      </Badge>
                    </td>
                    <td className="px-5 py-3 text-gray-600">
                      {s.trainingHoursYTD.toFixed(1)} / {s.trainingHoursRequired} hrs
                    </td>
                    <td className="px-5 py-3">
                      <Badge
                        variant={
                          s.complianceStatus === 'compliant'
                            ? 'compliant'
                            : s.complianceStatus === 'warning'
                              ? 'warning'
                              : 'critical'
                        }
                      >
                        {s.complianceStatus}
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
