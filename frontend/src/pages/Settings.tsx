import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  CreditCard,
  Building,
  AlertTriangle,
  ScrollText,
  Download,
  Trash2,
  Loader2,
  CheckCircle2,
  XCircle,
  Clock,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import Modal from '@/components/common/Modal';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { toast } from '@/components/common/Toast';
import { providersApi } from '@/api/providers';
import { dataExportApi, type DataExport } from '@/api/dataExport';
import { useSession } from '@/hooks/useSession';

export default function Settings(): JSX.Element {
  const user = useSession((s) => s.user);
  const { data: provider, isLoading, isError, refetch } = useQuery({
    queryKey: ['provider'],
    queryFn: () => providersApi.getProvider(),
  });

  return (
    <div>
      <PageHeader
        title="Settings"
        description="Manage your facility information and account."
      />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card
          title="Facility"
          description="Address, capacity, and licensing basics."
        >
          {isLoading ? (
            <LoadingSpinner />
          ) : isError ? (
            <div className="flex items-start gap-2 text-critical-700">
              <AlertTriangle className="h-5 w-5 mt-0.5" />
              <div>
                <div className="font-medium">Couldn't load your facility</div>
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
          ) : provider ? (
            <dl className="text-sm space-y-2">
              <Row label="Name" value={provider.name} icon={<Building className="h-4 w-4" />} />
              <Row label="State" value={provider.stateCode} />
              <Row
                label="License type"
                value={provider.licenseType === 'center' ? 'Child Care Center' : 'Family Home'}
              />
              <Row label="License #" value={provider.licenseNumber ?? '—'} />
              <Row
                label="Address"
                value={`${provider.address1}, ${provider.city}, ${provider.stateRegion} ${provider.postalCode}`}
              />
              <Row label="Capacity" value={String(provider.capacity)} />
            </dl>
          ) : null}
        </Card>

        <Card title="Account">
          <dl className="text-sm space-y-2">
            <Row label="Email" value={user?.email ?? '—'} />
            <Row label="Name" value={user?.fullName ?? '—'} />
            <Row label="Role" value={user?.role ?? '—'} />
          </dl>
        </Card>

        <Card
          title="Billing"
          description="Subscription and invoices."
          action={
            <Link to="/settings/billing">
              <Button size="sm" variant="secondary" leftIcon={<CreditCard className="h-4 w-4" />}>
                Manage
              </Button>
            </Link>
          }
        >
          <p className="text-sm text-gray-600">
            View your plan, update payment method, and access past invoices.
          </p>
        </Card>

        <Card
          title="Audit log"
          description="Track every change made to your facility data."
          action={
            <Link to="/settings/audit-log">
              <Button size="sm" variant="secondary" leftIcon={<ScrollText className="h-4 w-4" />}>
                View audit log
              </Button>
            </Link>
          }
        >
          <p className="text-sm text-gray-600">
            Admin-only. Events are retained for 7 years for compliance and
            inspection readiness.
          </p>
        </Card>

        <DataRetentionCard />
      </div>
    </div>
  );
}

// ---- Data & Retention ------------------------------------------------------

function DataRetentionCard(): JSX.Element {
  const qc = useQueryClient();
  const { data: exports, isLoading: listLoading, refetch: refetchList } = useQuery({
    queryKey: ['data-exports'],
    queryFn: () => dataExportApi.list(),
    // Auto-poll while any export is in flight so the UI updates without a manual refresh.
    refetchInterval: (q) => {
      const list = (q.state.data ?? []) as DataExport[];
      return list.some((e) => e.status === 'requested' || e.status === 'running') ? 5_000 : false;
    },
  });

  const createExport = useMutation({
    mutationFn: () => dataExportApi.create(),
    onSuccess: () => {
      toast.success("Export requested. We'll email you when it's ready.");
      qc.invalidateQueries({ queryKey: ['data-exports'] });
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Could not start export';
      toast.error(msg);
    },
  });

  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [confirmText, setConfirmText] = useState('');

  const deleteProvider = useMutation({
    mutationFn: () => dataExportApi.deleteProvider(confirmText),
    onSuccess: () => {
      toast.success(
        'Account scheduled for deletion. You have 90 days to cancel by contacting support.',
      );
      setDeleteModalOpen(false);
      setConfirmText('');
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Could not schedule deletion';
      toast.error(msg);
    },
  });

  async function handleDownload(id: string) {
    try {
      const url = await dataExportApi.getDownloadUrl(id);
      window.location.assign(url);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Download link expired — try again';
      toast.error(msg);
    }
  }

  return (
    <Card
      title="Data & Retention"
      description="Export all your data, or request account deletion."
    >
      <div className="space-y-5">
        <div>
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1">
              <div className="text-sm font-medium text-gray-900">Export all my data</div>
              <p className="mt-1 text-xs text-gray-600">
                A ZIP containing every database record and uploaded file tied to
                your facility. We'll email you a download link when it's ready.
              </p>
            </div>
            <Button
              size="sm"
              variant="secondary"
              leftIcon={<Download className="h-4 w-4" />}
              loading={createExport.isPending}
              onClick={() => createExport.mutate()}
            >
              Export
            </Button>
          </div>

          {listLoading ? (
            <div className="mt-3"><LoadingSpinner /></div>
          ) : exports && exports.length > 0 ? (
            <ul className="mt-4 divide-y divide-gray-100 border border-gray-100 rounded-lg">
              {exports.map((e) => (
                <li key={e.id} className="flex items-center justify-between gap-3 px-3 py-2 text-xs">
                  <div className="flex items-center gap-2 min-w-0">
                    <ExportStatusBadge status={e.status} />
                    <span className="text-gray-600 truncate">
                      {new Date(e.started_at).toLocaleString()}
                    </span>
                  </div>
                  {e.status === 'completed' ? (
                    <button
                      type="button"
                      className="text-brand-600 hover:text-brand-700 font-medium"
                      onClick={() => handleDownload(e.id)}
                    >
                      Download
                    </button>
                  ) : e.status === 'failed' ? (
                    <span className="text-critical-600 truncate" title={e.error_text}>
                      Failed
                    </span>
                  ) : (
                    <button
                      type="button"
                      className="text-gray-500 hover:text-gray-700"
                      onClick={() => refetchList()}
                    >
                      Refresh
                    </button>
                  )}
                </li>
              ))}
            </ul>
          ) : null}
        </div>

        <div className="border-t border-gray-100 pt-4">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1">
              <div className="text-sm font-medium text-gray-900">
                Cancel subscription and request deletion
              </div>
              <p className="mt-1 text-xs text-gray-600">
                Sets a 90-day grace clock. During the grace window your data is
                frozen but recoverable by contacting support. After 90 days,
                every record and file is permanently deleted.
              </p>
            </div>
            <Button
              size="sm"
              variant="danger"
              leftIcon={<Trash2 className="h-4 w-4" />}
              onClick={() => setDeleteModalOpen(true)}
            >
              Delete
            </Button>
          </div>
        </div>
      </div>

      <Modal
        open={deleteModalOpen}
        onClose={() => {
          setDeleteModalOpen(false);
          setConfirmText('');
        }}
        title="Request account deletion"
        description="This starts a 90-day retention clock. After the grace period, your data is permanently purged."
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                setDeleteModalOpen(false);
                setConfirmText('');
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={confirmText !== 'DELETE'}
              loading={deleteProvider.isPending}
              onClick={() => deleteProvider.mutate()}
            >
              Schedule deletion
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <div className="flex items-start gap-2 rounded-md bg-critical-50 p-3 text-xs text-critical-800">
            <AlertTriangle className="h-4 w-4 mt-0.5 flex-shrink-0" />
            <div>
              <div className="font-semibold">This action starts a 90-day purge.</div>
              <div>
                Your subscription is canceled at the end of the billing period.
                During the 90 days, sign in to Settings and contact support to
                cancel the deletion. After 90 days, recovery is impossible.
              </div>
            </div>
          </div>
          <Input
            label="Type DELETE to confirm"
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            placeholder="DELETE"
            autoFocus
          />
        </div>
      </Modal>
    </Card>
  );
}

function ExportStatusBadge({ status }: { status: DataExport['status'] }): JSX.Element {
  switch (status) {
    case 'completed':
      return (
        <span className="inline-flex items-center gap-1 text-brand-700">
          <CheckCircle2 className="h-3.5 w-3.5" />
          Ready
        </span>
      );
    case 'failed':
      return (
        <span className="inline-flex items-center gap-1 text-critical-700">
          <XCircle className="h-3.5 w-3.5" />
          Failed
        </span>
      );
    case 'running':
      return (
        <span className="inline-flex items-center gap-1 text-gray-700">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          Building
        </span>
      );
    case 'requested':
    default:
      return (
        <span className="inline-flex items-center gap-1 text-gray-500">
          <Clock className="h-3.5 w-3.5" />
          Queued
        </span>
      );
  }
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
