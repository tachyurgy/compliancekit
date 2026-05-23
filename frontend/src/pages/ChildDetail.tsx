import { Link, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  Mail,
  Syringe,
  FileText,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Send,
} from 'lucide-react';
import { useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import MessageHistory from '@/components/messages/MessageHistory';
import { useChild } from '@/hooks/useChildren';
import { childrenApi } from '@/api/children';
import { toast } from '@/components/common/Toast';
import { formatDate, fullName, formatPhone } from '@/lib/format';

function DocStatusIcon({
  status,
}: {
  status: 'missing' | 'pending' | 'complete' | 'expired';
}): JSX.Element {
  if (status === 'complete') return <CheckCircle2 className="h-4 w-4 text-brand-600" />;
  if (status === 'pending') return <Clock className="h-4 w-4 text-caution-500" />;
  return <AlertTriangle className="h-4 w-4 text-critical-600" />;
}

export default function ChildDetail(): JSX.Element {
  const { id } = useParams<{ id: string }>();
  const { data, isLoading, isError, error, refetch } = useChild(id);
  const [sendingLink, setSendingLink] = useState(false);

  const handleSendPortal = async () => {
    if (!id) return;
    setSendingLink(true);
    try {
      const { url } = await childrenApi.sendParentPortalLink(id);
      await navigator.clipboard.writeText(url).catch(() => undefined);
      toast.success('Parent portal link copied to clipboard.');
    } catch {
      toast.error('Could not generate portal link.');
    } finally {
      setSendingLink(false);
    }
  };

  if (isLoading) {
    return (
      <div className="py-10">
        <LoadingSpinner label="Loading child profile" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div>
        <Link
          to="/children"
          className="inline-flex items-center gap-1 text-sm font-medium text-ink-700 hover:text-ink-900 mb-5"
        >
          <ArrowLeft className="h-4 w-4" /> Back to children
        </Link>
        <div
          className="rounded-lb border border-critical-200 bg-critical-50/70 p-5 flex items-start gap-3"
          role="alert"
        >
          <AlertTriangle className="h-5 w-5 mt-0.5 text-critical-600 shrink-0" />
          <div className="flex-1">
            <div className="font-semibold text-critical-700">
              We couldn't load this child
            </div>
            <div className="text-sm text-ink-700 mt-1 leading-relaxed">
              {error instanceof Error
                ? error.message
                : 'The profile may have been removed.'}
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
      </div>
    );
  }

  return (
    <div>
      <Link
        to="/children"
        className="inline-flex items-center gap-1 text-sm font-medium text-ink-700 hover:text-ink-900 mb-5"
      >
        <ArrowLeft className="h-4 w-4" /> Back to children
      </Link>
      <PageHeader
        title={fullName(data.firstName, data.lastName)}
        description={`Born ${formatDate(data.dateOfBirth)}${
          data.enrollmentDate ? ` · Enrolled ${formatDate(data.enrollmentDate)}` : ''
        }`}
        actions={
          <Button
            leftIcon={<Send className="h-4 w-4" />}
            loading={sendingLink}
            onClick={handleSendPortal}
          >
            Send a reminder to {firstWord(data.parentName ?? '') || 'parent'}
          </Button>
        }
      />

      {/* Allergy strip — coral on cream per brand-voice.md, calm not alarming.
          Only rendered when allergies are present so it doesn't take up
          valuable real estate on every child page. */}
      {(data as { allergies?: string }).allergies && (
        <div
          className="mb-5 rounded-lb bg-critical-50/80 px-4 py-3 ring-1 ring-critical-200 flex items-start gap-2.5"
          role="note"
        >
          <AlertTriangle
            className="h-4 w-4 mt-0.5 text-critical-600 shrink-0"
            aria-hidden
          />
          <span className="text-sm font-medium text-critical-700">
            Allergies — {(data as { allergies?: string }).allergies}
          </span>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card title="Parent contact">
          <dl className="space-y-2 text-sm">
            <Row label="Name" value={data.parentName ?? '—'} />
            <Row
              label="Email"
              value={
                data.parentEmail ? (
                  <a className="text-brand-700" href={`mailto:${data.parentEmail}`}>
                    {data.parentEmail}
                  </a>
                ) : (
                  '—'
                )
              }
              icon={<Mail className="h-3.5 w-3.5" />}
            />
            <Row label="Phone" value={formatPhone(data.parentPhone) || '—'} />
          </dl>
        </Card>

        <Card
          title="Immunizations"
          description="State-required vaccines and exemptions."
          className="lg:col-span-2"
        >
          {data.immunizations.length === 0 ? (
            <p className="text-sm text-ink-500">No immunization records on file.</p>
          ) : (
            <ul className="divide-y divide-line-soft">
              {data.immunizations.map((i) => (
                <li key={i.vaccine} className="py-2.5 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Syringe className="h-4 w-4 text-ink-400" />
                    <div>
                      <div className="text-sm text-ink-900">{i.vaccine}</div>
                      <div className="text-xs text-ink-500">
                        {i.lastDose
                          ? `Last dose ${formatDate(i.lastDose)}`
                          : 'No dose recorded'}
                      </div>
                    </div>
                  </div>
                  <Badge
                    variant={
                      i.status === 'up_to_date'
                        ? 'compliant'
                        : i.status === 'due_soon'
                          ? 'warning'
                          : i.status === 'overdue'
                            ? 'critical'
                            : 'neutral'
                    }
                  >
                    {i.status === 'up_to_date'
                      ? 'Up to date'
                      : i.status === 'due_soon'
                        ? 'Due soon'
                        : i.status === 'overdue'
                          ? 'Overdue'
                          : 'Exempt'}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <div className="mt-6">
        <MessageHistory
          recipientKind="parent"
          recipientId={id!}
          composerHref={`/messages/new?parentId=${id}`}
        />
      </div>

      <div className="mt-6">
        <Card title="Required documents" description="What the state inspector expects on file.">
          {data.requiredDocs.length === 0 ? (
            <p className="text-sm text-ink-500">No required documents configured.</p>
          ) : (
            <ul className="divide-y divide-line-soft">
              {data.requiredDocs.map((doc) => (
                <li
                  key={doc.id}
                  className="py-3 flex items-start justify-between gap-4"
                >
                  <div className="flex items-start gap-3">
                    <DocStatusIcon status={doc.status} />
                    <div>
                      <div className="text-sm font-medium text-ink-900">{doc.label}</div>
                      <div className="text-xs text-ink-500">
                        <FileText className="inline h-3 w-3 mr-1" />
                        {doc.documentType}
                        {doc.dueDate && <span> · due {formatDate(doc.dueDate)}</span>}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge
                      variant={
                        doc.status === 'complete'
                          ? 'compliant'
                          : doc.status === 'pending'
                            ? 'warning'
                            : 'critical'
                      }
                    >
                      {doc.status === 'complete'
                        ? 'On file'
                        : doc.status === 'pending'
                          ? 'Awaiting upload'
                          : doc.status === 'expired'
                            ? 'Expired'
                            : 'Missing'}
                    </Badge>
                    {doc.documentId && (
                      <Link to={`/documents/${doc.documentId}`}>
                        <Button size="sm" variant="ghost">
                          Open
                        </Button>
                      </Link>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </div>
  );
}

function firstWord(s: string): string {
  const i = s.indexOf(' ');
  return i < 0 ? s : s.slice(0, i);
}

function Row({
  label,
  value,
  icon,
}: {
  label: string;
  value: React.ReactNode;
  icon?: React.ReactNode;
}): JSX.Element {
  return (
    <div className="flex items-center justify-between gap-4">
      <dt className="text-ink-500 inline-flex items-center gap-1.5">
        {icon}
        {label}
      </dt>
      <dd className="text-ink-900">{value}</dd>
    </div>
  );
}
