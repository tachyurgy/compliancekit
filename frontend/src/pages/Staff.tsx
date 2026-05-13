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

// REQ-004 / REQ-012d staff list page. The status pill is the most important
// element on the screen — we surface it in its own column. Filter by role
// and status, search by name. "Add a staff member" CTA lives top-right.

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

const CHANNEL_LABEL: Record<string, string> = {
  email: 'Email',
  sms: 'Text',
  both: 'Email + text',
};

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

  const hasFilter = Boolean(query || roleFilter || statusFilter);

  return (
    <div>
      <PageHeader
        eyebrow="Personnel files"
        title="Staff"
        description="Your team. We'll keep their files inspection-ready."
        actions={
          <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setAddOpen(true)}>
            Add a staff member
          </Button>
        }
      />

      <div className="mb-5 flex flex-wrap gap-3 items-end">
        <div className="max-w-sm flex-1 min-w-[200px]">
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
              We couldn't load your staff just now
            </div>
            <div className="text-sm text-ink-700 mt-1 leading-relaxed">
              {error instanceof Error
                ? error.message
                : 'Check your internet and try again in a moment.'}
            </div>
            <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
              Try again
            </Button>
          </div>
        </div>
      ) : visible.length === 0 ? (
        <EmptyState
          icon={<Users className="h-6 w-6" />}
          title={hasFilter ? 'No matches' : 'No staff yet'}
          description={
            hasFilter
              ? 'No staff match that search or filter. Try a different one.'
              : "Add your first staff member to get started. We'll keep their files inspection-ready."
          }
          action={
            !hasFilter ? (
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
                <tr className="text-left border-b border-line-soft text-xs uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-6 py-3.5 font-semibold">Name</th>
                  <th className="px-6 py-3.5 font-semibold">Role</th>
                  <th className="px-6 py-3.5 font-semibold">Status</th>
                  <th className="px-6 py-3.5 font-semibold">Phone</th>
                  <th className="px-6 py-3.5 font-semibold">Email</th>
                  <th className="px-6 py-3.5 font-semibold">Channel</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {visible.map((s) => {
                  const worstCount =
                    s.rollup.expired ||
                    s.rollup.missing ||
                    s.rollup.expiringSoon ||
                    s.rollup.awaitingUpload ||
                    0;
                  return (
                    <tr key={s.id} className="hover:bg-surface-sunken/50 transition-colors">
                      <td className="px-6 py-3.5">
                        <Link
                          to={`/staff/${s.id}`}
                          className="font-medium text-ink-900 hover:text-brand-700"
                        >
                          {fullName(s.firstName, s.lastName)}
                        </Link>
                      </td>
                      <td className="px-6 py-3.5 text-ink-700">
                        {ROLE_LABEL[s.role] ?? s.role.replace(/_/g, ' ')}
                      </td>
                      <td className="px-6 py-3.5">
                        <StatusPillBadge status={s.rollup.status} count={worstCount} />
                      </td>
                      <td className="px-6 py-3.5 text-ink-700">
                        {formatPhone(s.phone) || <span className="text-ink-400">—</span>}
                      </td>
                      <td className="px-6 py-3.5 text-ink-700">
                        {s.email || <span className="text-ink-400">—</span>}
                      </td>
                      <td className="px-6 py-3.5 text-ink-500">
                        {CHANNEL_LABEL[s.preferredChannel] ?? s.preferredChannel}
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

// AddStaffModal — "+ Add a staff member" dialog. Keeps form-state local;
// on success invalidates the list query and shows a calm toast.
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
      setSubmitError(
        err instanceof Error
          ? err.message
          : 'Something went wrong on our end. Please try again in a minute.',
      );
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
      <form id="add-staff-form" onSubmit={onSubmit} className="grid grid-cols-2 gap-4" noValidate>
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
          <div
            className="col-span-2 rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
            role="alert"
          >
            {submitError}
          </div>
        )}
      </form>
    </Modal>
  );
}
