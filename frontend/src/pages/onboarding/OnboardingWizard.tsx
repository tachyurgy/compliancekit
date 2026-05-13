import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { Check } from 'lucide-react';

import BrandMark from '@/components/common/BrandMark';
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
              className={`h-8 w-8 rounded-full text-xs font-semibold grid place-items-center transition-colors ${
                complete
                  ? 'bg-brand-600 text-white shadow-[inset_0_-1px_0_rgba(0,0,0,0.12)]'
                  : active
                    ? 'bg-brand-50 text-brand-700 ring-2 ring-brand-600'
                    : 'bg-surface-sunken text-ink-500'
              }`}
            >
              {complete ? <Check className="h-4 w-4" /> : idx + 1}
            </div>
            <span
              className={`text-sm whitespace-nowrap ${
                active ? 'text-ink-900 font-medium' : 'text-ink-500'
              }`}
            >
              {step.label}
            </span>
            {idx < WIZARD_STEPS.length - 1 && (
              <span className="mx-2 h-px w-8 bg-line hidden md:block" />
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
      <header className="bg-white border-b border-line">
        <div className="max-w-5xl mx-auto px-6 h-16 flex items-center">
          <BrandMark to="/" size="md" />
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
