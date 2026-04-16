import { Navigate, Route, Routes, useLocation, Link } from 'react-router-dom';
import { ShieldCheck, Check } from 'lucide-react';

import StepState from './steps/StepState';
import StepLicenseType from './steps/StepLicenseType';
import StepFacility from './steps/StepFacility';
import StepStaff from './steps/StepStaff';
import StepChildren from './steps/StepChildren';
import StepReview from './steps/StepReview';
import { WIZARD_STEPS } from './wizardStore';

function ProgressBar(): JSX.Element {
  const location = useLocation();
  const currentPath = location.pathname.split('/').pop() ?? 'state';
  const currentIndex = Math.max(
    0,
    WIZARD_STEPS.findIndex((s) => s.path === currentPath),
  );
  return (
    <ol className="flex items-center gap-2 overflow-x-auto">
      {WIZARD_STEPS.map((step, idx) => {
        const active = idx === currentIndex;
        const complete = idx < currentIndex;
        return (
          <li key={step.path} className="flex items-center gap-2">
            <div
              className={`h-7 w-7 rounded-full text-xs font-medium grid place-items-center ${
                complete
                  ? 'bg-brand-600 text-white'
                  : active
                    ? 'bg-brand-50 text-brand-700 ring-2 ring-brand-600'
                    : 'bg-gray-100 text-gray-500'
              }`}
            >
              {complete ? <Check className="h-3.5 w-3.5" /> : idx + 1}
            </div>
            <span
              className={`text-sm whitespace-nowrap ${
                active ? 'text-gray-900 font-medium' : 'text-gray-500'
              }`}
            >
              {step.label}
            </span>
            {idx < WIZARD_STEPS.length - 1 && (
              <span className="mx-2 h-px w-8 bg-gray-200 hidden md:block" />
            )}
          </li>
        );
      })}
    </ol>
  );
}

export default function OnboardingWizard(): JSX.Element {
  return (
    <div className="min-h-screen bg-surface-muted">
      <header className="bg-white border-b border-gray-100">
        <div className="max-w-5xl mx-auto px-6 h-16 flex items-center">
          <Link to="/" className="flex items-center gap-2">
            <ShieldCheck className="h-6 w-6 text-brand-600" />
            <span className="font-semibold">ComplianceKit</span>
          </Link>
        </div>
      </header>
      <div className="max-w-3xl mx-auto px-6 py-8">
        <div className="mb-8">
          <ProgressBar />
        </div>
        <Routes>
          <Route index element={<Navigate to="state" replace />} />
          <Route path="state" element={<StepState />} />
          <Route path="license" element={<StepLicenseType />} />
          <Route path="facility" element={<StepFacility />} />
          <Route path="staff" element={<StepStaff />} />
          <Route path="children" element={<StepChildren />} />
          <Route path="review" element={<StepReview />} />
          <Route path="*" element={<Navigate to="state" replace />} />
        </Routes>
      </div>
    </div>
  );
}
