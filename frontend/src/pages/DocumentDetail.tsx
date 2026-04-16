import { Link, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowLeft,
  Download,
  AlertTriangle,
  ExternalLink,
  Send,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { documentsApi } from '@/api/documents';
import { formatBytes, formatDate, formatTimestamp } from '@/lib/format';

export default function DocumentDetail(): JSX.Element {
  const { id } = useParams<{ id: string }>();
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['document', id],
    queryFn: () => documentsApi.get(id!),
    enabled: !!id,
  });

  if (isLoading) {
    return (
      <div className="py-10">
        <LoadingSpinner label="Loading document" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div>
        <Link to="/documents" className="inline-flex items-center gap-1 text-sm text-gray-600 mb-4">
          <ArrowLeft className="h-4 w-4" /> Back to documents
        </Link>
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load document</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Please try again.'}
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

  return (
    <div>
      <Link
        to="/documents"
        className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 mb-4"
      >
        <ArrowLeft className="h-4 w-4" /> Back to documents
      </Link>
      <PageHeader
        title={data.label}
        description={`${data.documentType.replace(/_/g, ' ')} · Uploaded ${formatTimestamp(data.uploadedAt)}`}
        actions={
          <>
            {data.downloadUrl && (
              <a href={data.downloadUrl} target="_blank" rel="noreferrer">
                <Button variant="secondary" leftIcon={<Download className="h-4 w-4" />}>
                  Download
                </Button>
              </a>
            )}
          </>
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card title="Details">
          <dl className="text-sm space-y-2">
            <Row label="Status">
              <Badge
                variant={
                  data.status === 'signed'
                    ? 'compliant'
                    : data.status === 'pending_signature'
                      ? 'warning'
                      : data.status === 'expired' || data.status === 'rejected'
                        ? 'critical'
                        : 'neutral'
                }
              >
                {data.status.replace(/_/g, ' ')}
              </Badge>
            </Row>
            <Row label="File">{data.fileName ?? '—'}</Row>
            <Row label="Size">{data.sizeBytes != null ? formatBytes(data.sizeBytes) : '—'}</Row>
            <Row label="Expires">
              {data.expirationDate ? formatDate(data.expirationDate) : '—'}
            </Row>
            <Row label="Subject">{data.subjectType}</Row>
          </dl>
        </Card>

        <Card
          title="Preview"
          description="In-browser preview. Use Download for the original."
          className="lg:col-span-2"
        >
          {data.previewUrl ? (
            <iframe
              src={data.previewUrl}
              title={data.label}
              className="w-full h-[560px] rounded-lg border border-gray-100 bg-white"
            />
          ) : (
            <div className="h-[320px] rounded-lg border border-dashed border-gray-200 grid place-items-center text-sm text-gray-500">
              No preview available for this file type.
            </div>
          )}
        </Card>
      </div>

      {data.signatureRequests.length > 0 && (
        <Card title="Signature requests" className="mt-6">
          <ul className="divide-y divide-gray-100">
            {data.signatureRequests.map((sr) => (
              <li key={sr.id} className="py-3 flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-medium text-gray-900">
                    {sr.signerName ?? sr.signerEmail}
                  </div>
                  <div className="text-xs text-gray-500">
                    Sent {formatTimestamp(sr.sentAt)}
                    {sr.signedAt && <span> · Signed {formatTimestamp(sr.signedAt)}</span>}
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <Badge
                    variant={
                      sr.status === 'signed'
                        ? 'compliant'
                        : sr.status === 'pending'
                          ? 'warning'
                          : 'critical'
                    }
                  >
                    {sr.status}
                  </Badge>
                  {sr.signUrl && sr.status === 'pending' && (
                    <a href={sr.signUrl} target="_blank" rel="noreferrer">
                      <Button size="sm" variant="ghost" rightIcon={<ExternalLink className="h-3.5 w-3.5" />}>
                        Open signing page
                      </Button>
                    </a>
                  )}
                  {sr.status === 'pending' && (
                    <Button size="sm" variant="secondary" leftIcon={<Send className="h-3.5 w-3.5" />}>
                      Resend
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }): JSX.Element {
  return (
    <div className="flex items-center justify-between gap-4">
      <dt className="text-gray-500">{label}</dt>
      <dd className="text-gray-900">{children}</dd>
    </div>
  );
}
