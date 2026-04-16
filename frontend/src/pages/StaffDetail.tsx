import { Link, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  Award,
  AlertTriangle,
  Send,
  CheckCircle2,
  Clock,
} from 'lucide-react';
import { useState } from 'react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { useStaffMember } from '@/hooks/useStaff';
import { staffApi } from '@/api/staff';
import { toast } from '@/components/common/Toast';
import { formatDate, formatPhone, formatRelativeDays, fullName } from '@/lib/format';

export default function StaffDetail(): JSX.Element {
  const { id } = useParams<{ id: string }>();
  const { data, isLoading, isError, error, refetch } = useStaffMember(id);
  const [sending, setSending] = useState(false);

  const handleSendPortal = async () => {
    if (!id) return;
    setSending(true);
    try {
      const { url } = await staffApi.sendStaffPortalLink(id);
      await navigator.clipboard.writeText(url).catch(() => undefined);
      toast.success('Staff portal link copied to clipboard.');
    } catch {
      toast.error('Could not generate portal link.');
    } finally {
      setSending(false);
    }
  };

  if (isLoading) {
    return (
      <div className="py-10">
        <LoadingSpinner label="Loading staff profile" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div>
        <Link to="/staff" className="inline-flex items-center gap-1 text-sm text-gray-600 mb-4">
          <ArrowLeft className="h-4 w-4" /> Back to staff
        </Link>
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load this staff member</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Try again shortly.'}
              </div>
              <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
                Retry
              </Button>
            </div>
          </div>
        </Card>
      </div>
    );
  }

  const trainingPct = Math.min(
    100,
    data.trainingHoursRequired > 0
      ? Math.round((data.trainingHoursYTD / data.trainingHoursRequired) * 100)
      : 0,
  );

  return (
    <div>
      <Link
        to="/staff"
        className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 mb-4"
      >
        <ArrowLeft className="h-4 w-4" /> Back to staff
      </Link>
      <PageHeader
        title={fullName(data.firstName, data.lastName)}
        description={`${data.role.replace(/_/g, ' ')}${
          data.hireDate ? ` · Hired ${formatDate(data.hireDate)}` : ''
        }`}
        actions={
          <Button
            leftIcon={<Send className="h-4 w-4" />}
            loading={sending}
            onClick={handleSendPortal}
          >
            Send staff portal link
          </Button>
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card title="Contact">
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between gap-4">
              <dt className="text-gray-500">Email</dt>
              <dd>{data.email ?? '—'}</dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-gray-500">Phone</dt>
              <dd>{formatPhone(data.phone) || '—'}</dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-gray-500">Background check</dt>
              <dd>
                <Badge
                  variant={
                    data.backgroundCheckStatus === 'cleared'
                      ? 'compliant'
                      : data.backgroundCheckStatus === 'pending'
                        ? 'warning'
                        : 'critical'
                  }
                >
                  {data.backgroundCheckStatus}
                </Badge>
              </dd>
            </div>
            {data.backgroundCheckDate && (
              <div className="flex justify-between gap-4">
                <dt className="text-gray-500">Checked on</dt>
                <dd>{formatDate(data.backgroundCheckDate)}</dd>
              </div>
            )}
          </dl>
        </Card>

        <Card title="Training" className="lg:col-span-2">
          <div className="text-sm text-gray-600">
            {data.trainingHoursYTD.toFixed(1)} of {data.trainingHoursRequired} required hours
            completed this year.
          </div>
          <div className="mt-3 h-2 rounded-full bg-gray-100 overflow-hidden">
            <div
              className={`h-full ${
                trainingPct >= 100
                  ? 'bg-brand-600'
                  : trainingPct >= 50
                    ? 'bg-caution-500'
                    : 'bg-critical-600'
              }`}
              style={{ width: `${trainingPct}%` }}
            />
          </div>
        </Card>
      </div>

      <div className="mt-6 grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card title="Certifications" description="Expiration dates are tracked automatically.">
          {data.certifications.length === 0 ? (
            <p className="text-sm text-gray-500">No certifications on file.</p>
          ) : (
            <ul className="divide-y divide-gray-100">
              {data.certifications.map((c) => (
                <li key={c.id} className="py-3 flex items-center justify-between gap-3">
                  <div className="flex items-center gap-3">
                    <Award className="h-4 w-4 text-gray-400" />
                    <div>
                      <div className="text-sm font-medium text-gray-900">{c.name}</div>
                      {c.expirationDate && (
                        <div className="text-xs text-gray-500">
                          Expires {formatDate(c.expirationDate)} (
                          {formatRelativeDays(c.expirationDate)})
                        </div>
                      )}
                    </div>
                  </div>
                  <Badge
                    variant={
                      c.status === 'valid'
                        ? 'compliant'
                        : c.status === 'expiring_soon'
                          ? 'warning'
                          : 'critical'
                    }
                  >
                    {c.status.replace(/_/g, ' ')}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card
          title="Required by your state"
          description="What your state requires for this role."
        >
          <ul className="divide-y divide-gray-100">
            {data.requiredCertifications.map((req) => (
              <li key={req.slug} className="py-2.5 flex items-center justify-between">
                <div className="flex items-center gap-2 text-sm text-gray-800">
                  {req.present ? (
                    <CheckCircle2 className="h-4 w-4 text-brand-600" />
                  ) : (
                    <Clock className="h-4 w-4 text-caution-500" />
                  )}
                  {req.label}
                </div>
                <Badge variant={req.present ? 'compliant' : 'warning'}>
                  {req.present ? 'On file' : 'Missing'}
                </Badge>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </div>
  );
}
