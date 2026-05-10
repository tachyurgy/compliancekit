import { Link } from 'react-router-dom';
import { Users, Plus, AlertTriangle, Search } from 'lucide-react';
import { useMemo, useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import Card from '@/components/common/Card';
import EmptyState from '@/components/common/EmptyState';
import StatusPillBadge from '@/components/common/StatusPill';
import Select from '@/components/common/Select';
import Modal from '@/components/common/Modal';
import { useStaffList, useCreateStaff } from '@/hooks/useStaff';
import { ROLE_LABEL, type StaffListRow } from '@/api/staff';
import { fullName, formatPhone } from '@/lib/format';
import { toast } from '@/components/common/Toast';

// REQ-004 staff list page. The status pill is the most important element
// on the screen — we surface it prominently in its own column. Filtering
// by role and status, search by name. The "Add a staff member" CTA lives
// top-right as the primary brand button.

const ROLE_OPTIONS = [
  { value: '', label: 'All roles' },
  ...Object.entries(ROLE_LABEL).map(([value, label]) => ({ value, label })),
];

const STATUS_OPTIONS = [
  { value: '', label: 'All staff' },
  { value: 'passing', label: 'Passing only' },
  { value: 'missing', label: 'Missing or awaiting' },
  { value: 'expired', label: 'Expired or expiring' },
];

export default function Staff(): JSX.Element {
  const [statusFilter, setStatusFilter] = useState<'' | 'passing' | 'missing' | 'expired'>('');
  const [roleFilter, setRoleFilter] = useState<string>('');
  const [query, setQuery] = useState('');
  const [addOpen, setAddOpen] = useState(false);

  const { data: staff = [], isLoading, isError, error, refetch } = useStaffList(
    statusFilter ? { status: statusFilter } : undefined,
  );

  const visible = useMemo(() => {
    let rows = staff as StaffListRow[];
    if (roleFilter) rows = rows.filter((s) => s.role === roleFilter);
    if (query.trim()) {
      const q = query.toLowerCase();
      rows = rows.filter((s) => fullName(s.firstName, s.lastName).toLowerCase().includes(q));
    }
    return rows;
  }, [staff, roleFilter, query]);

  return (
    <div>
      <PageHeader
        title="Staff"
        description="Your team. We'll keep their files inspection-ready."
        actions={
          <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setAddOpen(true)}>
            Add a staff member
          </Button>
        }
      />

      <div className="mb-4 flex flex-wrap gap-3 items-end">
        <div className="max-w-sm flex-1 min-w-[180px]">
          <Input
            placeholder="Search by name"
            leftIcon={<Search className="h-4 w-4" />}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <Select
          label="Role"
          value={roleFilter}
          onChange={(e) => setRoleFilter(e.target.value)}
          options={ROLE_OPTIONS}
        />
        <Select
          label="Status"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
          options={STATUS_OPTIONS}
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
              <div className="font-medium">We couldn't load your staff just now.</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Please try again.'}
              </div>
              <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
                Try again
              </Button>
            </div>
          </div>
        </Card>
      ) : visible.length === 0 ? (
        <EmptyState
          icon={<Users className="h-5 w-5 text-gray-500" />}
          title="No staff yet"
          description={
            query || roleFilter || statusFilter
              ? 'No matches. Try a different search or filter.'
              : "Add your first staff member to get started. We'll keep their files inspection-ready."
          }
          action={
            !query && !roleFilter && !statusFilter ? (
              <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setAddOpen(true)}>
                Add a staff member
              </Button>
            ) : undefined
          }
        />
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-gray-100 text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-5 py-3 font-medium">Name</th>
                  <th className="px-5 py-3 font-medium">Role</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Phone</th>
                  <th className="px-5 py-3 font-medium">Email</th>
                  <th className="px-5 py-3 font-medium">Channel</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {visible.map((s) => {
                  const worstCount =
                    s.rollup.expired ||
                    s.rollup.missing ||
                    s.rollup.expiringSoon ||
                    s.rollup.awaitingUpload ||
                    0;
                  return (
                    <tr key={s.id} className="hover:bg-surface-muted">
                      <td className="px-5 py-3">
                        <Link
                          to={`/staff/${s.id}`}
                          className="font-medium text-gray-900 hover:text-brand-700"
                        >
                          {fullName(s.firstName, s.lastName)}
                        </Link>
                      </td>
                      <td className="px-5 py-3 text-gray-600">
                        {ROLE_LABEL[s.role] ?? s.role.replace(/_/g, ' ')}
                      </td>
                      <td className="px-5 py-3">
                        <StatusPillBadge status={s.rollup.status} count={worstCount} />
                      </td>
                      <td className="px-5 py-3 text-gray-600">{formatPhone(s.phone) || '—'}</td>
                      <td className="px-5 py-3 text-gray-600">{s.email || '—'}</td>
                      <td className="px-5 py-3 text-gray-500 capitalize">
                        {s.preferredChannel}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      <AddStaffModal open={addOpen} onClose={() => setAddOpen(false)} />
    </div>
  );
}

// AddStaffModal is the "+ Add a staff member" dialog. Keeps form-state
// local; on success invalidates the list query and shows a calm toast.
function AddStaffModal({ open, onClose }: { open: boolean; onClose: () => void }): JSX.Element {
  const { mutateAsync, isPending } = useCreateStaff();
  const [first, setFirst] = useState('');
  const [last, setLast] = useState('');
  const [role, setRole] = useState('lead_teacher');
  const [hireDate, setHireDate] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [channel, setChannel] = useState<'email' | 'sms' | 'both'>('email');
  const [submitError, setSubmitError] = useState('');

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitError('');
    try {
      await mutateAsync({
        first_name: first.trim(),
        last_name: last.trim(),
        email: email.trim(),
        phone: phone.trim() || undefined,
        role,
        hire_date: hireDate || undefined,
        preferred_contact_channel: channel,
      });
      toast.success(`${first} ${last} added.`);
      onClose();
      setFirst('');
      setLast('');
      setEmail('');
      setPhone('');
      setHireDate('');
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong on our end.');
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Add a staff member"
      description="We'll start tracking their file the moment you save."
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={isPending}>
            Cancel
          </Button>
          <Button form="add-staff-form" type="submit" loading={isPending}>
            Save and continue
          </Button>
        </>
      }
    >
      <form id="add-staff-form" onSubmit={onSubmit} className="grid grid-cols-2 gap-3" noValidate>
        <Input
          label="First name"
          value={first}
          onChange={(e) => setFirst(e.target.value)}
          required
        />
        <Input
          label="Last name"
          value={last}
          onChange={(e) => setLast(e.target.value)}
          required
        />
        <Select
          label="Role"
          value={role}
          onChange={(e) => setRole(e.target.value)}
          options={Object.entries(ROLE_LABEL).map(([value, label]) => ({ value, label }))}
        />
        <Input
          label="Hire date"
          type="date"
          value={hireDate}
          onChange={(e) => setHireDate(e.target.value)}
        />
        <div className="col-span-2">
          <Input
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </div>
        <div className="col-span-2">
          <Input
            label="Phone"
            type="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="(305) 555-0100"
          />
        </div>
        <div className="col-span-2">
          <Select
            label="Preferred channel"
            value={channel}
            onChange={(e) => setChannel(e.target.value as typeof channel)}
            options={[
              { value: 'email', label: 'Email' },
              { value: 'sms', label: 'Text message' },
              { value: 'both', label: 'Email + text' },
            ]}
          />
        </div>
        {submitError && (
          <div className="col-span-2 text-sm text-critical-700">{submitError}</div>
        )}
      </form>
    </Modal>
  );
}
