import { useNavigate } from 'react-router-dom';
import { MapPin } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import { useWizardStore, type StateCode } from '../wizardStore';

const OPTIONS: { value: StateCode; title: string; subtitle: string }[] = [
  { value: 'CA', title: 'California', subtitle: 'Community Care Licensing — Title 22' },
  { value: 'TX', title: 'Texas', subtitle: 'HHSC Minimum Standards — Chapter 746' },
  { value: 'FL', title: 'Florida', subtitle: 'DCF Child Care Handbook — CFOP 170-20' },
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
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => pick(opt.value)}
              className={`text-left p-4 rounded-lg border transition-colors ${
                active
                  ? 'border-brand-600 bg-brand-50 ring-2 ring-brand-600/20'
                  : 'border-gray-200 hover:border-gray-300'
              }`}
            >
              <MapPin
                className={`h-5 w-5 mb-2 ${active ? 'text-brand-600' : 'text-gray-400'}`}
              />
              <div className="font-medium text-gray-900">{opt.title}</div>
              <div className="text-xs text-gray-500 mt-0.5">{opt.subtitle}</div>
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
