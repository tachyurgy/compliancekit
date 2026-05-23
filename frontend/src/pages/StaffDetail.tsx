import { Link, useParams } from 'react-router-dom';
import { ArrowLeft, AlertTriangle, Send, Copy, ExternalLink, FileText, Archive, Pencil } from 'lucide-react';
import { useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import Modal from '@/components/common/Modal';
import StatusPillBadge from '@/components/common/StatusPill';
import Select from '@/components/common/Select';
import Input from '@/components/common/Input';
import MessageHistory from '@/components/messages/MessageHistory';
import { useStaffMember, useArchiveStaff, useUpdateStaff } from '@/hooks/useStaff';
import { staffApi, ROLE_LABEL, type RequirementStatus, type StaffDetail as StaffDetailT } from '@/api/staff';
import { toast } from '@/components/common/Toast';
import { fullName, formatDate, formatPhone } from '@/lib/format';

// REQ-004 staff detail page — the screen operators live on. The "Send a
// reminder" button anchors the header so it's findable in <2 seconds.
// The required-documents section is the bullseye of the page; below it
// is "Other documents" and the activity timeline.

export default function StaffDetailPage(): JSX.Element {
  const { id } = useParams<{ id: string }>();
  const { data, isLoading, isError, error, refetch } = useStaffMember(id);
  const archive = useArchiveStaff();
  const [reminderOpen, setReminderOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [uploadLink, setUploadLink] = useState<string>('');

  async function refreshUploadLink() {
    if (!id) return;
    try {
      const r = await staffApi.getUploadLink(id);
      setUploadLink(r.url);
    } catch {
      toast.error('We couldn\'t generate the upload link. Try again.');
    }
  }

  if (isLoading) {
    return (
      <div className="py-10">
        <LoadingSpinner label="Loading staff file" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div>
        <Link
          to="/staff"
          className="inline-flex items-center gap-1 text-sm text-ink-700 hover:text-ink-900 mb-5"
        >
          <ArrowLeft className="h-4 w-4" /> Back to staff
        </Link>
        <div
          className="rounded-lb border border-critical-200 bg-critical-50/70 p-5 flex items-start gap-3"
          role="alert"
        >
          <AlertTriangle className="h-5 w-5 mt-0.5 text-critical-600 shrink-0" />
          <div className="flex-1">
            <div className="font-semibold text-critical-700">
              We couldn't load this staff member
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
      </div>
    );
  }

  const staff = data as StaffDetailT;
  const worstCount =
    staff.rollup.expired ||
    staff.rollup.missing ||
    staff.rollup.expiringSoon ||
    staff.rollup.awaitingUpload ||
    0;

  return (
    <div>
      <Link
        to="/staff"
        className="inline-flex items-center gap-1 text-sm font-medium text-ink-700 hover:text-ink-900 mb-5"
      >
        <ArrowLeft className="h-4 w-4" /> Back to staff
      </Link>

      <PageHeader
        title={fullName(staff.firstName, staff.lastName)}
        description={
          <span className="flex items-center gap-3 text-sm text-ink-700">
            <span>{ROLE_LABEL[staff.role] ?? staff.role.replace(/_/g, ' ')}</span>
            {staff.hireDate && <span>· Hired {formatDate(staff.hireDate)}</span>}
            <StatusPillBadge status={staff.rollup.status} count={worstCount} size="sm" />
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              leftIcon={<Pencil className="h-4 w-4" />}
              onClick={() => setEditOpen(true)}
            >
              Edit
            </Button>
            <Button leftIcon={<Send className="h-4 w-4" />} onClick={() => setReminderOpen(true)}>
              Send a reminder
            </Button>
          </div>
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card title="Contact">
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between gap-4">
              <dt className="text-ink-500">Email</dt>
              <dd>{staff.email || '—'}</dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-ink-500">Phone</dt>
              <dd>{formatPhone(staff.phone) || '—'}</dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-ink-500">Preferred channel</dt>
              <dd className="capitalize">{staff.preferredChannel}</dd>
            </div>
            {staff.archivedAt && (
              <div className="flex justify-between gap-4 pt-2 border-t border-line-soft">
                <dt className="text-ink-500">Archived</dt>
                <dd>{formatDate(staff.archivedAt)}</dd>
              </div>
            )}
          </dl>
          {!staff.archivedAt && (
            <Button
              size="sm"
              variant="ghost"
              className="mt-3"
              leftIcon={<Archive className="h-4 w-4" />}
              loading={archive.isPending}
              onClick={async () => {
                if (!id) return;
                if (!window.confirm(`Archive ${staff.firstName}? You can still see their file later.`)) return;
                await archive.mutateAsync(id);
                toast.success(`${staff.firstName} archived.`);
              }}
            >
              Archive this person
            </Button>
          )}
        </Card>

        <Card title="What Sarah sees on her phone" className="lg:col-span-2">
          <p className="text-sm text-ink-700">
            Your staff member can upload from their phone using this private link. We never
            email it on its own — share it from this screen so you stay in control.
          </p>
          <div className="mt-3 flex gap-2 items-center">
            <Input
              readOnly
              value={uploadLink || 'Click below to generate a link…'}
              className="flex-1"
            />
            <Button size="sm" variant="secondary" onClick={refreshUploadLink}>
              {uploadLink ? 'Refresh' : 'Generate'}
            </Button>
            {uploadLink && (
              <>
                <Button
                  size="sm"
                  variant="ghost"
                  leftIcon={<Copy className="h-4 w-4" />}
                  onClick={async () => {
                    await navigator.clipboard.writeText(uploadLink).catch(() => undefined);
                    toast.success('Copied to clipboard.');
                  }}
                >
                  Copy
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  leftIcon={<ExternalLink className="h-4 w-4" />}
                  onClick={() => window.open(uploadLink, '_blank')}
                >
                  Open
                </Button>
              </>
            )}
          </div>
        </Card>
      </div>

      <div className="mt-6 grid grid-cols-1 gap-6">
        <Card
          title="Required documents"
          description={`${staff.rollup.onFile} of ${staff.rollup.total} on file`}
          padded={false}
        >
          {staff.requiredDocuments.length === 0 ? (
            <div className="px-5 py-6 text-sm text-ink-500">
              No required documents are configured for this role yet.
            </div>
          ) : (
            <ul className="divide-y divide-line-soft">
              {staff.requiredDocuments.map((req) => (
                <RequirementRow
                  key={req.docKind}
                  req={req}
                  staffId={id!}
                  staffFirstName={staff.firstName}
                />
              ))}
            </ul>
          )}
        </Card>

        {staff.otherDocuments.length > 0 && (
          <Card title="Other documents" padded={false}>
            <ul className="divide-y divide-line-soft">
              {staff.otherDocuments.map((d) => (
                <li key={d.id} className="px-5 py-3 flex justify-between gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <FileText className="h-4 w-4 text-ink-400" />
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-ink-900 truncate">{d.title || d.kind}</div>
                      <div className="text-xs text-ink-500">
                        {d.expiresOn ? `Expires ${formatDate(d.expiresOn)}` : 'No expiration'}
                        {d.source ? ` · ${d.source}` : ''}
                      </div>
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </Card>
        )}

        <Card title="Activity">
          {staff.activity.length === 0 ? (
            <p className="text-sm text-ink-500">Nothing here yet. Uploads and reminders will show up as they happen.</p>
          ) : (
            <ul className="divide-y divide-line-soft">
              {staff.activity.map((ev) => (
                <li key={ev.id} className="py-2.5 text-sm flex justify-between gap-4">
                  <div>
                    <div className="text-ink-900">{ev.summary}</div>
                    {ev.detail && <div className="text-xs text-ink-500">{ev.detail}</div>}
                  </div>
                  <div className="text-xs text-ink-500 whitespace-nowrap">
                    {formatDate(ev.createdAt)}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {/* REQ-040: per-staff message log */}
      <div className="mt-6">
        <MessageHistory
          recipientKind="staff"
          recipientId={id!}
          composerHref={`/messages/new?staffId=${id}`}
        />
      </div>

      <SendReminderModal
        open={reminderOpen}
        onClose={() => setReminderOpen(false)}
        staffId={id!}
        staffFirstName={staff.firstName}
        defaultChannel={staff.preferredChannel}
        requirements={staff.requiredDocuments}
      />

      <EditStaffModal
        open={editOpen}
        onClose={() => setEditOpen(false)}
        staffId={id!}
        initial={staff}
      />
    </div>
  );
}

// REQ-040: Edit modal that wraps the existing staffApi.update mutation.
// Binds to name, role, email, phone, hire date, and preferred channel —
// the fields a director would actually want to fix from this screen.
function EditStaffModal({
  open,
  onClose,
  staffId,
  initial,
}: {
  open: boolean;
  onClose: () => void;
  staffId: string;
  initial: StaffDetailT;
}): JSX.Element {
  const update = useUpdateStaff(staffId);
  const [firstName, setFirstName] = useState(initial.firstName);
  const [lastName, setLastName] = useState(initial.lastName);
  const [role, setRole] = useState(initial.role);
  const [email, setEmail] = useState(initial.email);
  const [phone, setPhone] = useState(initial.phone);
  const [hireDate, setHireDate] = useState(initial.hireDate ?? '');
  const [preferredChannel, setPreferredChannel] = useState<'email' | 'sms' | 'both'>(initial.preferredChannel);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await update.mutateAsync({
        first_name: firstName,
        last_name: lastName,
        role,
        email,
        phone,
        hire_date: hireDate || undefined,
        preferred_contact_channel: preferredChannel,
      });
      toast.success('Updated.');
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "We couldn't save that.");
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Edit staff profile"
      description="Update the basics. We'll keep the document history intact."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={update.isPending}>
            Cancel
          </Button>
          <Button form="edit-staff-form" type="submit" loading={update.isPending}>
            Save changes
          </Button>
        </>
      }
    >
      <form id="edit-staff-form" onSubmit={submit} className="space-y-3" noValidate>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Input label="First name" value={firstName} onChange={(e) => setFirstName(e.target.value)} required />
          <Input label="Last name" value={lastName} onChange={(e) => setLastName(e.target.value)} required />
        </div>
        <Select
          label="Role"
          value={role}
          onChange={(e) => setRole(e.target.value)}
          options={[
            { value: 'owner', label: 'Owner' },
            { value: 'director', label: 'Director' },
            { value: 'lead_teacher', label: 'Lead teacher' },
            { value: 'assistant_teacher', label: 'Assistant teacher' },
            { value: 'aide', label: 'Aide' },
            { value: 'cook', label: 'Cook' },
            { value: 'driver', label: 'Driver' },
            { value: 'volunteer', label: 'Volunteer' },
          ]}
        />
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Input
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Input label="Phone" value={phone} onChange={(e) => setPhone(e.target.value)} />
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Input
            label="Hire date"
            type="date"
            value={hireDate}
            onChange={(e) => setHireDate(e.target.value)}
          />
          <Select
            label="Preferred channel"
            value={preferredChannel}
            onChange={(e) => setPreferredChannel(e.target.value as 'email' | 'sms' | 'both')}
            options={[
              { value: 'email', label: 'Email' },
              { value: 'sms', label: 'Text' },
              { value: 'both', label: 'Email + text' },
            ]}
          />
        </div>
      </form>
    </Modal>
  );
}

// One row in the "Required documents" section. Surfaces the pill, dates,
// source label, and the three actions (View / Replace / Send a reminder)
// per spec.
function RequirementRow({
  req,
  staffId,
  staffFirstName,
}: {
  req: RequirementStatus;
  staffId: string;
  staffFirstName: string;
}): JSX.Element {
  const [sending, setSending] = useState(false);

  async function nag() {
    setSending(true);
    try {
      await staffApi.sendReminder(staffId, {
        reason: req.status === 'missing' ? 'missing' : 'manual',
        doc_kind: req.docKind,
      });
      toast.success(`Reminder sent to ${staffFirstName}.`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'We couldn\'t send the reminder. Try again.');
    } finally {
      setSending(false);
    }
  }

  const faded = req.status === 'missing';
  return (
    <li className={`px-5 py-3 flex flex-wrap items-center gap-3 justify-between ${faded ? 'opacity-80' : ''}`}>
      <div className="min-w-0">
        <div className="text-sm font-medium text-ink-900">{req.label}</div>
        <div className="text-xs text-ink-500 mt-0.5">
          {req.expiresOn ? `Expires ${formatDate(req.expiresOn)}` : ''}
          {req.expiresOn && req.source ? ' · ' : ''}
          {req.source}
          {!req.expiresOn && !req.source && faded ? `Missing — request from ${staffFirstName}` : ''}
        </div>
      </div>
      <div className="flex items-center gap-3">
        <StatusPillBadge status={req.status} size="sm" />
        <Button size="sm" variant="ghost" loading={sending} onClick={nag}>
          Send a reminder
        </Button>
      </div>
    </li>
  );
}

// "Send a reminder" modal. Defaults to the first missing/expired/expiring
// doc, falls back to "any doc"; defaults channel to the staff's preferred.
function SendReminderModal({
  open,
  onClose,
  staffId,
  staffFirstName,
  defaultChannel,
  requirements,
}: {
  open: boolean;
  onClose: () => void;
  staffId: string;
  staffFirstName: string;
  defaultChannel: 'email' | 'sms' | 'both';
  requirements: RequirementStatus[];
}): JSX.Element {
  const fixable = requirements.filter((r) => r.status !== 'on_file');
  const [docKind, setDocKind] = useState<string>(fixable[0]?.docKind ?? '');
  const [note, setNote] = useState('');
  const [sending, setSending] = useState(false);

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setSending(true);
    try {
      await staffApi.sendReminder(staffId, {
        reason: 'manual',
        doc_kind: docKind || undefined,
        note: note || undefined,
      });
      toast.success(`Reminder sent to ${staffFirstName} via ${defaultChannel === 'both' ? 'email + text' : defaultChannel}.`);
      onClose();
      setNote('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'We couldn\'t send the reminder. Try again.');
    } finally {
      setSending(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Send a reminder"
      description={`We'll reach out to ${staffFirstName} via ${defaultChannel === 'both' ? 'email and text' : defaultChannel}.`}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={sending}>
            Cancel
          </Button>
          <Button form="send-reminder-form" type="submit" loading={sending}>
            Send a reminder
          </Button>
        </>
      }
    >
      <form id="send-reminder-form" onSubmit={send} className="space-y-3" noValidate>
        <Select
          label="What's the reminder about?"
          value={docKind}
          onChange={(e) => setDocKind(e.target.value)}
          options={[
            { value: '', label: 'Anything missing or expiring' },
            ...fixable.map((r) => ({ value: r.docKind, label: r.label })),
          ]}
        />
        <div>
          <label className="block text-sm font-medium text-ink-800 mb-1.5">
            Add a note <span className="text-ink-400 font-normal">(optional)</span>
          </label>
          <textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={3}
            className="block w-full rounded-lb-sm border border-line px-3.5 py-2.5 text-sm placeholder:text-ink-400 focus:border-brand-600 focus:ring-2 focus:ring-brand-600/20 focus:outline-none"
            placeholder="Hi Sarah — your CPR card expires next month. Snap a photo of the new one when you have a sec."
          />
        </div>
      </form>
    </Modal>
  );
}
