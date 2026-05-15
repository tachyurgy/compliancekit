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
import FacilityProfileBanner from '@/components/dashboard/FacilityProfileBanner';
import ScoreHero, { ScoreHeroSkeleton } from '@/components/dashboard/ScoreHero';
import AttendanceStrip from '@/components/dashboard/AttendanceStrip';
import ShaynaTour from '@/components/tour/ShaynaTour';
import { useDashboard } from '@/hooks/useDashboard';
import { useScore } from '@/hooks/useScore';
import { useQuery } from '@tanstack/react-query';
import { demoApi } from '@/api/demo';
import { formatDate, formatRelativeDays } from '@/lib/format';

const SEVERITY_DOT: Record<'critical' | 'warning' | 'info', string> = {
  critical: 'bg-critical-600',
  warning: 'bg-caution-500',
  info: 'bg-ink-400',
};

const SEVERITY_ICON: Record<'critical' | 'warning' | 'info', string> = {
  critical: 'text-critical-600',
  warning: 'text-caution-600',
  info: 'text-ink-500',
};

export default function Dashboard(): JSX.Element {
  const { data, isLoading, isError, error, refetch } = useDashboard();
  const score = useScore();
  const demoStatus = useQuery({
    queryKey: ['demo-status'],
    queryFn: () => demoApi.status(),
    staleTime: 30_000,
  });

  return (
    <div>
      <ShaynaTour hasDemoData={demoStatus.data?.has_demo_data ?? false} />
      <TrialBanner />
      <FacilityProfileBanner />
      <PageHeader
        eyebrow="Today at your center"
        title="Welcome back."
        description="Anything flagged below needs action before your next inspection. Tap a card to fast-forward."
        actions={
          <>
            <Link to="/children">
              <Button variant="secondary" size="sm" leftIcon={<Baby className="h-4 w-4" />}>
                Add a child
              </Button>
            </Link>
            <Link to="/staff">
              <Button variant="secondary" size="sm" leftIcon={<UserPlus className="h-4 w-4" />}>
                Add a staff member
              </Button>
            </Link>
            <Link to="/documents">
              <Button size="sm" leftIcon={<FilePlus className="h-4 w-4" />}>
                Upload a document
              </Button>
            </Link>
          </>
        }
      />

      {/* REQ-016 / REQ-012c: today's warnings + absence cards. Always above
          the score hero because they are the most time-sensitive surface. */}
      <AttendanceStrip />

      {/* REQ-013 / REQ-012c: live score hero. Loads independently of
          /api/dashboard so a slow rules eval can't block the most important
          glance on the page. */}
      {score.isLoading ? (
        <ScoreHeroSkeleton />
      ) : score.data ? (
        <ScoreHero data={score.data} />
      ) : null}

      <div className="mt-7" />

      {isLoading ? (
        <ScoreHeroSkeleton />
      ) : isError ? (
        <div
          className="rounded-lb border border-critical-200 bg-critical-50/70 p-5 flex items-start gap-3"
          role="alert"
        >
          <AlertTriangle className="h-5 w-5 mt-0.5 text-critical-600 shrink-0" />
          <div className="flex-1">
            <div className="font-semibold text-critical-700">
              We couldn't load your dashboard
            </div>
            <div className="text-sm text-ink-700 mt-1 leading-relaxed">
              {error instanceof Error
                ? error.message
                : "Check your internet and try again in a moment."}
            </div>
            <Button className="mt-3" size="sm" variant="secondary" onClick={() => refetch()}>
              Try again
            </Button>
          </div>
        </div>
      ) : data ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <Card
            title="Critical alerts"
            description="These need action now or could lead to a citation."
          >
            {data.alerts.length === 0 ? (
              <EmptyState
                icon={<ShieldCheck className="h-6 w-6" />}
                title="You're inspection-ready"
                description="No critical alerts right now. Keep it up — we'll surface anything new the moment it lands."
              />
            ) : (
              <ul className="divide-y divide-line-soft -mx-1">
                {data.alerts.map((alert) => {
                  const sev = (alert.severity ?? 'info') as 'critical' | 'warning' | 'info';
                  return (
                    <li key={alert.id} className="px-1 py-4 flex items-start gap-3">
                      <AlertTriangle
                        className={`h-5 w-5 mt-0.5 shrink-0 ${SEVERITY_ICON[sev]}`}
                        aria-hidden
                      />
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium text-ink-900">{alert.title}</div>
                        <div className="text-sm text-ink-700 mt-0.5 leading-relaxed">
                          {alert.description}
                        </div>
                        {alert.dueDate && (
                          <div className="mt-1.5 text-xs text-ink-500">
                            Due {formatDate(alert.dueDate)} ({formatRelativeDays(alert.dueDate)})
                          </div>
                        )}
                      </div>
                      {alert.href && (
                        <Link
                          to={alert.href}
                          className="text-brand-700 hover:text-brand-800 text-sm font-semibold inline-flex items-center gap-1 shrink-0"
                        >
                          Review
                          <ArrowRight className="h-3.5 w-3.5" />
                        </Link>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </Card>

          <Card
            title="90-day timeline"
            description="Upcoming deadlines, drills, and recertifications."
          >
            {data.timeline.length === 0 ? (
              <EmptyState
                icon={<Calendar className="h-6 w-6" />}
                title="Nothing coming up"
                description="You have no deadlines in the next 90 days. We're keeping an eye on it."
              />
            ) : (
              <ol className="relative border-l-2 border-line pl-5 space-y-5">
                {data.timeline.map((item) => {
                  const sev = (item.severity ?? 'info') as 'critical' | 'warning' | 'info';
                  return (
                    <li key={item.id} className="relative">
                      <span
                        className={`absolute -left-[27px] top-1 h-3.5 w-3.5 rounded-full ring-2 ring-white ${SEVERITY_DOT[sev]}`}
                        aria-hidden
                      />
                      <div className="text-xs text-ink-500">{formatDate(item.date)}</div>
                      <div className="text-sm font-medium text-ink-900 mt-0.5">
                        {item.label}
                      </div>
                      <div className="text-xs text-ink-500 mt-0.5">{item.category}</div>
                      {item.href && (
                        <Link
                          to={item.href}
                          className="mt-1.5 inline-flex items-center gap-1 text-xs font-semibold text-brand-700 hover:text-brand-800"
                        >
                          Open
                          <ArrowRight className="h-3 w-3" />
                        </Link>
                      )}
                    </li>
                  );
                })}
              </ol>
            )}
          </Card>
        </div>
      ) : (
        <LoadingSpinner />
      )}
    </div>
  );
}
