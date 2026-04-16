import { useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FileText, Filter, Upload, AlertTriangle } from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import EmptyState from '@/components/common/EmptyState';
import Select from '@/components/common/Select';
import { documentsApi, type DaycareDocument } from '@/api/documents';
import { formatDate, formatBytes } from '@/lib/format';
import { toast } from '@/components/common/Toast';
import { ApiError } from '@/api/client';

function statusVariant(status: DaycareDocument['status']) {
  if (status === 'signed') return 'compliant';
  if (status === 'pending_signature') return 'warning';
  if (status === 'expired' || status === 'rejected') return 'critical';
  return 'neutral';
}

export default function Documents(): JSX.Element {
  const qc = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [subjectFilter, setSubjectFilter] = useState<string>('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const { data: documents = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ['documents', { statusFilter, subjectFilter }],
    queryFn: () =>
      documentsApi.list({
        status: (statusFilter || undefined) as DaycareDocument['status'] | undefined,
        subjectType:
          (subjectFilter || undefined) as DaycareDocument['subjectType'] | undefined,
      }),
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) =>
      documentsApi.upload(file, { documentType: 'other', subjectType: 'provider' }),
    onSuccess: () => {
      toast.success('Document uploaded.');
      void qc.invalidateQueries({ queryKey: ['documents'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : 'Upload failed.');
    },
  });

  const grouped = useMemo(() => {
    const byType = new Map<string, DaycareDocument[]>();
    for (const doc of documents) {
      const arr = byType.get(doc.documentType) ?? [];
      arr.push(doc);
      byType.set(doc.documentType, arr);
    }
    return Array.from(byType.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [documents]);

  return (
    <div>
      <PageHeader
        title="Documents"
        description="All compliance documents. Filter by type or upload a new one."
        actions={
          <>
            <input
              ref={fileInputRef}
              type="file"
              className="sr-only"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) uploadMutation.mutate(f);
                e.target.value = '';
              }}
            />
            <Button
              leftIcon={<Upload className="h-4 w-4" />}
              loading={uploadMutation.isPending}
              onClick={() => fileInputRef.current?.click()}
            >
              Upload
            </Button>
          </>
        }
      />

      <Card padded className="mb-4">
        <div className="flex items-center gap-3 flex-wrap">
          <div className="flex items-center gap-2 text-sm text-gray-500">
            <Filter className="h-4 w-4" /> Filters
          </div>
          <div className="w-40">
            <Select
              options={[
                { value: '', label: 'All statuses' },
                { value: 'uploaded', label: 'Uploaded' },
                { value: 'pending_signature', label: 'Pending signature' },
                { value: 'signed', label: 'Signed' },
                { value: 'expired', label: 'Expired' },
                { value: 'rejected', label: 'Rejected' },
              ]}
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
            />
          </div>
          <div className="w-40">
            <Select
              options={[
                { value: '', label: 'All subjects' },
                { value: 'provider', label: 'Facility' },
                { value: 'child', label: 'Child' },
                { value: 'staff', label: 'Staff' },
              ]}
              value={subjectFilter}
              onChange={(e) => setSubjectFilter(e.target.value)}
            />
          </div>
        </div>
      </Card>

      {isLoading ? (
        <Card padded>
          <div className="space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="skeleton h-10 w-full" />
            ))}
          </div>
        </Card>
      ) : isError ? (
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load documents</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Please try again.'}
              </div>
              <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
                Retry
              </Button>
            </div>
          </div>
        </Card>
      ) : documents.length === 0 ? (
        <EmptyState
          icon={<FileText className="h-5 w-5 text-gray-500" />}
          title="No documents yet"
          description="Upload a policy, inspection report, or signed form to get started."
        />
      ) : (
        <div className="space-y-6">
          {grouped.map(([type, docs]) => (
            <Card key={type} title={type.replace(/_/g, ' ')} padded={false}>
              <ul className="divide-y divide-gray-100">
                {docs.map((doc) => (
                  <li key={doc.id} className="px-5 py-3 flex items-center justify-between">
                    <div>
                      <Link
                        to={`/documents/${doc.id}`}
                        className="text-sm font-medium text-gray-900 hover:text-brand-700"
                      >
                        {doc.label}
                      </Link>
                      <div className="text-xs text-gray-500">
                        {doc.fileName ?? 'No file'}
                        {doc.sizeBytes != null && (
                          <span> · {formatBytes(doc.sizeBytes)}</span>
                        )}
                        <span> · Uploaded {formatDate(doc.uploadedAt)}</span>
                        {doc.expirationDate && (
                          <span> · Expires {formatDate(doc.expirationDate)}</span>
                        )}
                      </div>
                    </div>
                    <Badge variant={statusVariant(doc.status)}>
                      {doc.status.replace(/_/g, ' ')}
                    </Badge>
                  </li>
                ))}
              </ul>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
