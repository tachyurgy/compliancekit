import { useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ShieldCheck,
  Upload,
  CheckCircle2,
  AlertTriangle,
  FileText,
  Clock,
} from 'lucide-react';

import LoadingSpinner from '@/components/common/LoadingSpinner';
import { portalApi } from '@/api/portal';
import ToastViewport, { toast } from '@/components/common/Toast';
import { isLikelyBase62Token } from '@/lib/base62';

export default function PortalStaff(): JSX.Element {
  const { token } = useParams<{ token: string }>();
  const qc = useQueryClient();
  const [uploadingId, setUploadingId] = useState<string | null>(null);

  const validToken = useMemo(() => !!token && isLikelyBase62Token(token), [token]);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['staff-portal', token],
    queryFn: () => portalApi.staffSession(token!),
    enabled: validToken,
  });

  const uploadMutation = useMutation({
    mutationFn: ({ file, requiredDocId }: { file: File; requiredDocId: string }) =>
      portalApi.staffUpload(token!, requiredDocId, file),
    onSuccess: () => {
      toast.success('Upload received.');
      void qc.invalidateQueries({ queryKey: ['staff-portal', token] });
    },
    onError: () => toast.error('Upload failed. Please try again.'),
    onSettled: () => setUploadingId(null),
  });

  if (!validToken) {
    return (
      <Shell>
        <ErrorState title="This link isn't valid" description="Please ask your employer for a new one." />
      </Shell>
    );
  }

  if (isLoading) {
    return (
      <Shell>
        <div className="py-10 grid place-items-center">
          <LoadingSpinner label="Loading your upload page" />
        </div>
      </Shell>
    );
  }

  if (isError || !data) {
    return (
      <Shell>
        <ErrorState
          title="This link has expired"
          description={
            error instanceof Error ? error.message : 'Ask your employer to send a new link.'
          }
        />
      </Shell>
    );
  }

  const remaining = data.requiredDocs.filter((d) => d.status !== 'complete').length;

  return (
    <Shell>
      <div className="text-center">
        <h1 className="text-xl font-semibold text-gray-900">
          Hi {data.staff.firstName}
        </h1>
        <p className="mt-1 text-sm text-gray-600">
          {data.providerName} needs a few certifications and forms on file.
        </p>
      </div>

      <div className="mt-6 card p-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          {remaining === 0 ? (
            <CheckCircle2 className="h-5 w-5 text-brand-600" />
          ) : (
            <Clock className="h-5 w-5 text-caution-500" />
          )}
          <span className="text-sm font-medium text-gray-900">
            {remaining === 0
              ? 'All items received.'
              : `${remaining} item${remaining === 1 ? '' : 's'} remaining`}
          </span>
        </div>
      </div>

      <ul className="mt-4 space-y-3">
        {data.requiredDocs.map((doc) => {
          const isUploading = uploadingId === doc.id && uploadMutation.isPending;
          return (
            <li key={doc.id} className="card p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-2">
                    <FileText className="h-4 w-4 text-gray-400" />
                    <div className="text-sm font-medium text-gray-900">
                      {doc.label}
                      {doc.required && <span className="text-critical-600">*</span>}
                    </div>
                  </div>
                  {doc.description && (
                    <p className="mt-1 text-xs text-gray-600">{doc.description}</p>
                  )}
                </div>
                <StatusChip status={doc.status} />
              </div>

              {doc.status !== 'complete' && (
                <label
                  className={`mt-3 inline-flex items-center gap-2 h-8 px-3 rounded-lg border border-gray-200 bg-white text-sm font-medium shadow-sm hover:bg-gray-50 cursor-pointer ${
                    isUploading ? 'opacity-60 pointer-events-none' : ''
                  }`}
                >
                  <input
                    type="file"
                    className="sr-only"
                    accept="image/*,application/pdf"
                    disabled={isUploading}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (!file) return;
                      setUploadingId(doc.id);
                      uploadMutation.mutate({ file, requiredDocId: doc.id });
                      e.target.value = '';
                    }}
                  />
                  <Upload className="h-4 w-4" />
                  <span>{isUploading ? 'Uploading...' : 'Upload'}</span>
                </label>
              )}
            </li>
          );
        })}
      </ul>
      <ToastViewport />
    </Shell>
  );
}

function Shell({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <div className="min-h-screen bg-surface-muted">
      <header className="bg-white border-b border-gray-100">
        <div className="max-w-xl mx-auto px-4 h-14 flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-brand-600" />
          <span className="font-semibold">ComplianceKit</span>
        </div>
      </header>
      <main className="max-w-xl mx-auto px-4 py-6">{children}</main>
    </div>
  );
}

function StatusChip({
  status,
}: {
  status: 'missing' | 'pending' | 'complete' | 'expired';
}): JSX.Element {
  const map = {
    complete: { label: 'Received', className: 'bg-brand-50 text-brand-700' },
    pending: { label: 'Review', className: 'bg-caution-50 text-caution-600' },
    missing: { label: 'Needed', className: 'bg-critical-50 text-critical-700' },
    expired: { label: 'Expired', className: 'bg-critical-50 text-critical-700' },
  } as const;
  const { label, className } = map[status];
  return (
    <span className={`inline-flex px-2 py-0.5 rounded-full text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function ErrorState({
  title,
  description,
}: {
  title: string;
  description: string;
}): JSX.Element {
  return (
    <div className="card p-6 text-center">
      <div className="mx-auto mb-3 h-10 w-10 rounded-full bg-critical-50 grid place-items-center">
        <AlertTriangle className="h-5 w-5 text-critical-600" />
      </div>
      <h1 className="text-lg font-semibold text-gray-900">{title}</h1>
      <p className="mt-1 text-sm text-gray-600">{description}</p>
    </div>
  );
}
