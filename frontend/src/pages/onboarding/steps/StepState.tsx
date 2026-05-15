import { useNavigate } from 'react-router-dom';
import { MapPin } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import { useWizardStore, type StateCode } from '../wizardStore';

// FL-only for the 2026-05-29 launch. CA + TX show as "coming soon" so we can
// keep the visual map intact without making the operator think they're picking
// a state we don't actually support yet.
interface StateOption {
  value: StateCode;
  title: string;
  subtitle: string;
  available: boolean;
}
const OPTIONS: StateOption[] = [
  { value: 'FL', title: 'Florida', subtitle: 'DCF Child Care Handbook — CFOP 170-20', available: true },
  { value: 'CA', title: 'California', subtitle: 'Coming soon — join the waitlist', available: false },
  { value: 'TX', title: 'Texas', subtitle: 'Coming soon — join the waitlist', available: false },
];

export default function StepState(): JSX.Element {
  const navigate = useNavigate();
  const stateCode = useWizardStore((s) => s.stateCode);
  const setField = useWizardStore((s) => s.setField);

  const pick = (code: StateCode) => {
    setField('stateCode', code);
    setField('stateRegion', code);
  };

  return (
    <Card
      title="Which state are you licensed in?"
      description="We only need one state to get you started. You can add locations in other states later."
    >
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        {OPTIONS.map((opt) => {
          const active = stateCode === opt.value;
          const disabled = !opt.available;
          return (
            <button
              key={opt.value}
              type="button"
              disabled={disabled}
              aria-disabled={disabled}
              onClick={() => !disabled && pick(opt.value)}
              className={`relative text-left p-4 rounded-lg border transition-colors ${
                disabled
                  ? 'border-line bg-surface-muted cursor-not-allowed opacity-60'
                  : active
                    ? 'border-brand-600 bg-brand-50 ring-2 ring-brand-600/20'
                    : 'border-line hover:border-brand-200'
              }`}
            >
              {disabled && (
                <span className="absolute top-2 right-2 text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded bg-ink-100 text-ink-600">
                  Soon
                </span>
              )}
              <MapPin
                className={`h-5 w-5 mb-2 ${active ? 'text-brand-600' : 'text-ink-400'}`}
              />
              <div className="font-medium text-ink-900">{opt.title}</div>
              <div className="text-xs text-ink-500 mt-0.5">{opt.subtitle}</div>
            </button>
          );
        })}
      </div>

      <div className="mt-6 flex justify-end">
        <Button disabled={!stateCode} onClick={() => navigate('/onboarding/license')}>
          Continue
        </Button>
      </div>
    </Card>
  );
}
