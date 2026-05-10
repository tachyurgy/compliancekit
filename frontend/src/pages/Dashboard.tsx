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
import Button from '@/components/common/Button';
import EmptyState from '@/components/common/EmptyState';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import TrialBanner from '@/components/common/TrialBanner';
import ScoreHero, { ScoreHeroSkeleton } from '@/components/dashboard/ScoreHero';
import { useDashboard } from '@/hooks/useDashboard';
import { useScore } from '@/hooks/useScore';
import { formatDate, formatRelativeDays } from '@/lib/format';

export default function Dashboard(): JSX.Element {
  const { data, isLoading, isError, error, refetch } = useDashboard();
  const score = useScore();

  return (
    <div>
      <TrialBanner />
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

      {/* REQ-013: live score hero. Loads independently of /api/dashboard so a
          slow rules eval can't block the most important glance on the page. */}
      {score.isLoading ? (
        <ScoreHeroSkeleton />
      ) : score.data ? (
        <ScoreHero data={score.data} />
      ) : null}

      <div className="mt-6" />

      {isLoading ? (
        <ScoreHeroSkeleton />
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
          {/* REQ-013: the score hero replaces the old ScoreRing/KPI grid.
              Below it we keep the critical-alerts list and 90-day timeline,
              which are still useful for the operator who wants to drill in. */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
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

