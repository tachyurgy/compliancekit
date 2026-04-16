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
        <Link to="/children" className="inline-flex items-center gap-1 text-sm text-gray-600 mb-4">
          <ArrowLeft className="h-4 w-4" /> Back to children
        </Link>
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load this child</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'The profile may have been removed.'}
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
      </div>
    );
  }

  return (
    <div>
      <Link
        to="/children"
        className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 mb-4"
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
            Send parent portal link
          </Button>
        }
      />

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
            <p className="text-sm text-gray-500">No immunization records on file.</p>
          ) : (
            <ul className="divide-y divide-gray-100">
              {data.immunizations.map((i) => (
                <li key={i.vaccine} className="py-2.5 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Syringe className="h-4 w-4 text-gray-400" />
                    <div>
                      <div className="text-sm text-gray-900">{i.vaccine}</div>
                      <div className="text-xs text-gray-500">
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
                    {i.status === 'exempt' ? 'Exempt' : i.status.replace(/_/g, ' ')}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <div className="mt-6">
        <Card title="Required documents" description="What the state inspector expects on file.">
          {data.requiredDocs.length === 0 ? (
            <p className="text-sm text-gray-500">No required documents configured.</p>
          ) : (
            <ul className="divide-y divide-gray-100">
              {data.requiredDocs.map((doc) => (
                <li
                  key={doc.id}
                  className="py-3 flex items-start justify-between gap-4"
                >
                  <div className="flex items-start gap-3">
                    <DocStatusIcon status={doc.status} />
                    <div>
                      <div className="text-sm font-medium text-gray-900">{doc.label}</div>
                      <div className="text-xs text-gray-500">
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
                      {doc.status}
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
      <dt className="text-gray-500 inline-flex items-center gap-1.5">
        {icon}
        {label}
      </dt>
      <dd className="text-gray-900">{value}</dd>
    </div>
  );
}
