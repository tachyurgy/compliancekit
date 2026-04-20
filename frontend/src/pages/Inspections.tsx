import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ClipboardCheck, Plus, AlertTriangle, CheckCircle2 } from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import EmptyState from '@/components/common/EmptyState';
import { inspectionsApi } from '@/api/inspections';
import type { InspectionRun } from '@/api/inspections';
import { ApiError } from '@/api/client';
import { toast } from '@/components/common/Toast';
import { formatTimestamp } from '@/lib/format';

function scoreVariant(score: number): 'compliant' | 'warning' | 'critical' {
  if (score >= 90) return 'compliant';
  if (score >= 70) return 'warning';
  return 'critical';
}

function progressLabel(run: InspectionRun): string {
  if (run.completed_at) {
    return 'Finalized';
  }
  return `${run.items_answered}/${run.total_items} answered`;
}

export default function Inspections(): JSX.Element {
  const navigate = useNavigate();
  const qc = useQueryClient();

  const { data: runs = [], isLoading, isError, error, refetch } = useQuery({
    queryKey: ['inspections'],
    queryFn: () => inspectionsApi.list(),
  });

  const start = useMutation({
    mutationFn: () => inspectionsApi.start(),
    onSuccess: (detail) => {
      toast.success('Mock inspection started. Good luck.');
      void qc.invalidateQueries({ queryKey: ['inspections'] });
      navigate(`/inspections/${detail.run.id}`);
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : 'Could not start inspection.');
    },
  });

  return (
    <div>
      <PageHeader
        title="Inspections"
        description="Walk the same checklist a state inspector uses. Same questions. Same order. Same rubric."
        actions={
          <Button
            leftIcon={<Plus className="h-4 w-4" />}
            loading={start.isPending}
            onClick={() => start.mutate()}
            size="lg"
          >
            Start a mock inspection
          </Button>
        }
      />

      {isLoading ? (
        <Card padded>
          <div className="space-y-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="skeleton h-12 w-full" />
            ))}
          </div>
        </Card>
      ) : isError ? (
        <Card padded>
          <div className="flex items-start gap-3 text-critical-700">
            <AlertTriangle className="h-5 w-5 mt-0.5" />
            <div>
              <div className="font-medium">Couldn't load inspections</div>
              <div className="text-sm text-gray-600">
                {error instanceof Error ? error.message : 'Try again in a moment.'}
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
      ) : runs.length === 0 ? (
        <EmptyState
          icon={<ClipboardCheck className="h-5 w-5 text-gray-500" />}
          title="You've never run a mock inspection"
          description="Stop hoping the inspector goes easy on you. Run one now — we'll score you with the same rubric the state uses."
          action={
            <Button
              size="lg"
              leftIcon={<Plus className="h-4 w-4" />}
              loading={start.isPending}
              onClick={() => start.mutate()}
            >
              Start a mock inspection
            </Button>
          }
        />
      ) : (
        <Card padded={false}>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-gray-100 text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-5 py-3 font-medium">Started</th>
                  <th className="px-5 py-3 font-medium">State</th>
                  <th className="px-5 py-3 font-medium">Progress</th>
                  <th className="px-5 py-3 font-medium">Score</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {runs.map((run) => (
                  <tr
                    key={run.id}
                    className="hover:bg-surface-muted cursor-pointer"
                    onClick={() => navigate(`/inspections/${run.id}`)}
                  >
                    <td className="px-5 py-3 text-gray-700">
                      {formatTimestamp(run.started_at)}
                    </td>
                    <td className="px-5 py-3 text-gray-600">
                      {run.state} &middot; {run.form_ref}
                    </td>
                    <td className="px-5 py-3 text-gray-600">{progressLabel(run)}</td>
                    <td className="px-5 py-3">
                      {run.score != null ? (
                        <Badge variant={scoreVariant(run.score)}>{run.score}</Badge>
                      ) : (
                        <span className="text-gray-400 text-xs">—</span>
                      )}
                    </td>
                    <td className="px-5 py-3">
                      {run.completed_at ? (
                        <Badge variant="compliant" icon={<CheckCircle2 className="h-3 w-3" />}>
                          Finalized
                        </Badge>
                      ) : (
                        <Badge variant="warning">In progress</Badge>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}
