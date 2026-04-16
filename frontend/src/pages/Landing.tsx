import { Link, Navigate } from 'react-router-dom';
import { ShieldCheck, CheckCircle2, AlertTriangle, FileCheck2 } from 'lucide-react';

import Button from '@/components/common/Button';
import { useSession } from '@/hooks/useSession';
import LoadingSpinner from '@/components/common/LoadingSpinner';

const FEATURES = [
  {
    icon: CheckCircle2,
    title: 'Real-time compliance score',
    body: 'See exactly where you stand against your state\'s licensing rules, updated the moment anything changes.',
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
        <LoadingSpinner label="Loading ComplianceKit" />
      </div>
    );
  }

  if (user) {
    return <Navigate to={user.onboardingComplete ? '/dashboard' : '/onboarding'} replace />;
  }

  return (
    <div className="min-h-screen bg-surface">
      <header className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
        <Link to="/" className="flex items-center gap-2">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold text-lg">ComplianceKit</span>
        </Link>
        <div className="flex items-center gap-2">
          <Link to="/login">
            <Button variant="ghost">Sign in</Button>
          </Link>
          <Link to="/login">
            <Button>Get started</Button>
          </Link>
        </div>
      </header>

      <section className="max-w-4xl mx-auto px-6 pt-16 pb-20 text-center">
        <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-brand-50 text-brand-700 text-xs font-medium ring-1 ring-inset ring-brand-200 mb-5">
          <ShieldCheck className="h-3.5 w-3.5" />
          Built for CA, TX, and FL child care providers
        </div>
        <h1 className="text-4xl sm:text-5xl font-semibold tracking-tight text-gray-900">
          Be inspection-ready every single day.
        </h1>
        <p className="mt-5 text-lg text-gray-600 max-w-2xl mx-auto">
          ComplianceKit replaces the paper binder, the spreadsheet, and the pre-inspection
          panic with a single dashboard that tracks every requirement, deadline, and
          document your state inspector will ask for.
        </p>
        <div className="mt-8 flex items-center justify-center gap-3">
          <Link to="/login">
            <Button size="lg">Start free trial</Button>
          </Link>
          <a href="/compliancekit-product-overview.html">
            <Button size="lg" variant="secondary">
              Learn more
            </Button>
          </a>
        </div>
      </section>

      <section className="max-w-6xl mx-auto px-6 pb-24 grid grid-cols-1 md:grid-cols-3 gap-6">
        {FEATURES.map((f) => (
          <div key={f.title} className="card p-6">
            <f.icon className="h-6 w-6 text-brand-600" />
            <h3 className="mt-3 font-semibold text-gray-900">{f.title}</h3>
            <p className="mt-2 text-sm text-gray-600">{f.body}</p>
          </div>
        ))}
      </section>

      <footer className="border-t border-gray-100 py-8 text-center text-xs text-gray-500">
        <p>
          &copy; {new Date().getFullYear()} ComplianceKit. Inspection-ready child care
          compliance.
        </p>
      </footer>
    </div>
  );
}
