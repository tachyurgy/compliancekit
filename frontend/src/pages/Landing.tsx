import { Link, Navigate } from 'react-router-dom';
import { CheckCircle2, AlertTriangle, FileCheck2 } from 'lucide-react';

import Button from '@/components/common/Button';
import DemoLaunchButton from '@/components/common/DemoLaunchButton';
import { ARCHITECTURE_URL, DEMO_MODE } from '@/lib/demo';
import BrandMark from '@/components/common/BrandMark';
import { useSession } from '@/hooks/useSession';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import DemoDeploymentNotice from '@/components/common/DemoDeploymentNotice';

const FEATURES = [
  {
    icon: CheckCircle2,
    title: 'Real-time compliance score',
    body: "See exactly where you stand against your state's licensing rules, updated the moment anything changes.",
  },
  {
    icon: AlertTriangle,
    title: 'Deadline engine',
    body: 'Immunization due dates, CPR recertifications, fire drills — all tracked with 90/60/30-day reminders.',
  },
  {
    icon: FileCheck2,
    title: 'Inspection-ready in a click',
    body: 'Generate the exact binder your state inspector expects, indexed and ordered to their checklist.',
  },
];

export default function Landing(): JSX.Element {
  const { user, status } = useSession();

  if (status === 'loading') {
    return (
      <div className="h-screen grid place-items-center">
        <LoadingSpinner label="Loading Levelbrook" />
      </div>
    );
  }

  if (user) {
    return <Navigate to={user.onboardingComplete ? '/dashboard' : '/onboarding'} replace />;
  }

  return (
    <div className="min-h-screen bg-surface-muted">
      <DemoDeploymentNotice />
      <header
        className="border-b border-line bg-white/85 backdrop-blur sticky top-0 z-50"
      >
        <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
          <BrandMark to="/" size="md" />
          <div className="flex items-center gap-2">
            <Link to="/login">
              <Button variant="ghost" size="sm">Sign in</Button>
            </Link>
            {DEMO_MODE ? null : (
              <Link to="/signup">
                <Button size="sm">Start free trial</Button>
              </Link>
            )}
          </div>
        </div>
      </header>

      <section
        className="border-b border-line"
        style={{
          backgroundImage:
            'radial-gradient(ellipse 90% 60% at 80% 0%, rgba(13,111,94,0.09), transparent 60%),' +
            'radial-gradient(ellipse 80% 50% at 0% 100%, rgba(197,119,74,0.09), transparent 60%)',
        }}
      >
        <div className="max-w-4xl mx-auto px-6 pt-20 pb-24 text-center">
          <span className="eyebrow">Florida — and where you grow next</span>
          <h1 className="mt-5 font-display text-5xl sm:text-6xl text-ink-900 tracking-tight leading-[1.02]">
            Be inspection-ready every single day.
          </h1>
          <p className="mt-6 text-lg text-ink-700 max-w-2xl mx-auto leading-relaxed">
            Levelbrook replaces the paper binder, the spreadsheet, and the
            pre-inspection panic with a single dashboard that tracks every
            requirement, deadline, and document your DCF inspector will ask for.
          </p>
          {DEMO_MODE ? (
            <>
              <div className="mt-9 flex items-center justify-center gap-3 flex-wrap">
                <DemoLaunchButton />
                <a href={ARCHITECTURE_URL}>
                  <Button size="lg" variant="secondary">
                    Read the architecture
                  </Button>
                </a>
              </div>
              <p className="mt-5 text-sm text-ink-500">
                Public demo · fictional data · email, SMS and payments are switched off
              </p>
            </>
          ) : (
            <>
              <div className="mt-9 flex items-center justify-center gap-3 flex-wrap">
                <Link to="/signup">
                  <Button size="lg">Start your free trial</Button>
                </Link>
                <a href="/compliancekit-product-overview.html">
                  <Button size="lg" variant="secondary">
                    See how it works
                  </Button>
                </a>
              </div>
              <p className="mt-5 text-sm text-ink-500">
                14 days free · No card hassle · Cancel any time
              </p>
            </>
          )}
        </div>
      </section>

      <section className="max-w-6xl mx-auto px-6 py-20 grid grid-cols-1 md:grid-cols-3 gap-6">
        {FEATURES.map((f) => (
          <div
            key={f.title}
            className="bg-white rounded-lb border border-line shadow-card p-7 transition-shadow hover:shadow-elevated"
          >
            <div
              className="h-11 w-11 rounded-lb-sm bg-brand-50 ring-1 ring-brand-100 grid place-items-center"
              aria-hidden
            >
              <f.icon className="h-5 w-5 text-brand-600" />
            </div>
            <h3 className="mt-5 font-semibold text-ink-900">{f.title}</h3>
            <p className="mt-2 text-sm text-ink-700 leading-relaxed">{f.body}</p>
          </div>
        ))}
      </section>

      <footer className="border-t border-line py-10 text-center text-xs text-ink-500">
        <p>
          &copy; {new Date().getFullYear()} Levelbrook. Inspection-ready child care
          compliance.
        </p>
      </footer>
    </div>
  );
}
