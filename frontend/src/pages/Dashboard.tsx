import { Link } from 'react-router-dom';
import {
  ShieldCheck,
  AlertTriangle,
  ArrowRight,
  Calendar,
  FilePlus,
  UserPlus,
  Baby,
} from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Badge from '@/components/common/Badge';
import Button from '@/components/common/Button';
import EmptyState from '@/components/common/EmptyState';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { useDashboard } from '@/hooks/useDashboard';
import { formatDate, formatRelativeDays, formatTimestamp } from '@/lib/format';

function ScoreRing({ score }: { score: number }): JSX.Element {
  const radius = 54;
  const circ = 2 * Math.PI * radius;
  const offset = circ - (score / 100) * circ;
  const color =
    score >= 90
      ? 'text-brand-600'
      : score >= 70
        ? 'text-caution-500'
        : 'text-critical-600';

  return (
    <div className="relative h-36 w-36 flex-shrink-0">
      <svg viewBox="0 0 128 128" className="h-full w-full -rotate-90">
        <circle
          cx="64"
          cy="64"
          r={radius}
          stroke="currentColor"
          strokeWidth="10"
          fill="none"
          className="text-gray-100"
        />
        <circle
          cx="64"
          cy="64"
          r={radius}
          stroke="currentColor"
          strokeWidth="10"
          fill="none"
          strokeLinecap="round"
          strokeDasharray={circ}
          strokeDashoffset={offset}
          className={color}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <div className="text-3xl font-semibold text-gray-900">{score}</div>
        <div className="text-xs text-gray-500">of 100</div>
      </div>
    </div>
  );
}

function HeroSkeleton(): JSX.Element {
  return (
    <div className="card p-6 flex items-center gap-6">
      <div className="skeleton h-36 w-36 rounded-full" />
      <div className="flex-1 space-y-3">
        <div className="skeleton h-5 w-1/3" />
        <div className="skeleton h-4 w-2/3" />
        <div className="skeleton h-4 w-1/2" />
      </div>
    </div>
  );
}

export default function Dashboard(): JSX.Element {
  const { data, isLoading, isError, error, refetch } = useDashboard();

  return (
    <div>
      <PageHeader
        title="Compliance dashboard"
        description="Your real-time readiness view. Anything flagged below needs action before your next inspection."
        actions={
          <>
            <Link to="/children">
              <Button variant="secondary" leftIcon={<Baby className="h-4 w-4" />}>
                Add child
              </Button>
            </Link>
            <Link to="/staff">
              <Button variant="secondary" leftIcon={<UserPlus className="h-4 w-4" />}>
                Add staff
              </Button>
            </Link>
            <Link to="/documents">
              <Button leftIcon={<FilePlus className="h-4 w-4" />}>Upload document</Button>
            </Link>
          </>
        }
      />

      {isLoading ? (
        <HeroSkeleton />
      ) : isError ? (
        <div className="card p-6 flex items-start gap-3 text-critical-700">
          <AlertTriangle className="h-5 w-5 mt-0.5" />
          <div>
            <div className="font-medium">Couldn't load your dashboard</div>
            <div className="text-sm text-gray-600">
              {error instanceof Error ? error.message : 'Try again in a moment.'}
            </div>
            <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
              Retry
            </Button>
          </div>
        </div>
      ) : data ? (
        <>
          <div className="card p-6 flex flex-col sm:flex-row items-start sm:items-center gap-6">
            <ScoreRing score={data.complianceScore} />
            <div className="flex-1">
              <div className="flex items-center gap-2">
                <ShieldCheck className="h-5 w-5 text-brand-600" />
                <h2 className="text-lg font-semibold text-gray-900">Compliance score</h2>
                <Badge
                  variant={
                    data.scoreDelta >= 0 ? 'compliant' : data.scoreDelta < -5 ? 'critical' : 'warning'
                  }
                >
                  {data.scoreDelta >= 0 ? '+' : ''}
                  {data.scoreDelta} pts
                </Badge>
              </div>
              <p className="mt-1 text-sm text-gray-600">
                Last updated {formatTimestamp(data.updatedAt)}. Based on {data.counts.children}{' '}
                children, {data.counts.staff} staff, and {data.counts.documents} documents.
              </p>
              <div className="mt-4 grid grid-cols-2 sm:grid-cols-4 gap-3">
                <KpiTile label="Critical alerts" value={data.counts.criticalAlerts} tone="critical" />
                <KpiTile label="Warnings" value={data.counts.warningAlerts} tone="warning" />
                <KpiTile label="Children" value={data.counts.children} />
                <KpiTile label="Staff" value={data.counts.staff} />
              </div>
            </div>
          </div>

          <div className="mt-6 grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Card
              title="Critical alerts"
              description="These need action now or could lead to a citation."
            >
              {data.alerts.length === 0 ? (
                <EmptyState
                  icon={<ShieldCheck className="h-5 w-5 text-brand-600" />}
                  title="No critical alerts"
                  description="You're inspection-ready. Keep it up."
                />
              ) : (
                <ul className="divide-y divide-gray-100">
                  {data.alerts.map((alert) => (
                    <li key={alert.id} className="py-3 flex items-start gap-3">
                      <div className="mt-0.5">
                        <AlertTriangle
                          className={`h-5 w-5 ${
                            alert.severity === 'critical'
                              ? 'text-critical-600'
                              : alert.severity === 'warning'
                                ? 'text-caution-500'
                                : 'text-gray-400'
                          }`}
                        />
                      </div>
                      <div className="flex-1">
                        <div className="text-sm font-medium text-gray-900">{alert.title}</div>
                        <div className="text-sm text-gray-600">{alert.description}</div>
                        {alert.dueDate && (
                          <div className="mt-1 text-xs text-gray-500">
                            Due {formatDate(alert.dueDate)} ({formatRelativeDays(alert.dueDate)})
                          </div>
                        )}
                      </div>
                      {alert.href && (
                        <Link
                          to={alert.href}
                          className="text-brand-700 hover:text-brand-800 text-sm font-medium inline-flex items-center gap-1"
                        >
                          Review
                          <ArrowRight className="h-3.5 w-3.5" />
                        </Link>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </Card>

            <Card
              title="90-day timeline"
              description="Upcoming deadlines, drills, and recertifications."
            >
              {data.timeline.length === 0 ? (
                <EmptyState
                  icon={<Calendar className="h-5 w-5 text-gray-500" />}
                  title="Nothing coming up"
                  description="You have no deadlines in the next 90 days."
                />
              ) : (
                <ol className="relative border-l border-gray-100 pl-5 space-y-4">
                  {data.timeline.map((item) => (
                    <li key={item.id} className="relative">
                      <span
                        className={`absolute -left-[23px] top-1 h-3 w-3 rounded-full ring-2 ring-white ${
                          item.severity === 'critical'
                            ? 'bg-critical-600'
                            : item.severity === 'warning'
                              ? 'bg-caution-500'
                              : 'bg-gray-400'
                        }`}
                      />
                      <div className="text-xs text-gray-500">{formatDate(item.date)}</div>
                      <div className="text-sm font-medium text-gray-900">{item.label}</div>
                      <div className="text-xs text-gray-500">{item.category}</div>
                      {item.href && (
                        <Link
                          to={item.href}
                          className="mt-1 inline-flex items-center gap-1 text-xs text-brand-700 hover:text-brand-800"
                        >
                          Open
                          <ArrowRight className="h-3 w-3" />
                        </Link>
                      )}
                    </li>
                  ))}
                </ol>
              )}
            </Card>
          </div>
        </>
      ) : (
        <LoadingSpinner />
      )}
    </div>
  );
}

function KpiTile({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: 'critical' | 'warning';
}): JSX.Element {
  const color =
    tone === 'critical' ? 'text-critical-600' : tone === 'warning' ? 'text-caution-600' : 'text-gray-900';
  return (
    <div className="rounded-lg bg-surface-muted px-3 py-2">
      <div className={`text-xl font-semibold ${color}`}>{value}</div>
      <div className="text-xs text-gray-600">{label}</div>
    </div>
  );
}
