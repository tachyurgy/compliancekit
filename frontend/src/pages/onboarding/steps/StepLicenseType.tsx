import { useNavigate } from 'react-router-dom';
import { Building2, Home } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import { useWizardStore, type LicenseType } from '../wizardStore';

const OPTIONS: { value: LicenseType; title: string; subtitle: string; icon: typeof Building2 }[] = [
  {
    value: 'center',
    title: 'Child Care Center',
    subtitle: 'Commercial facility, 13+ children typically',
    icon: Building2,
  },
  {
    value: 'family_home',
    title: 'Family Child Care Home',
    subtitle: 'In-home, small group (usually <= 12 children)',
    icon: Home,
  },
];

export default function StepLicenseType(): JSX.Element {
  const navigate = useNavigate();
  const licenseType = useWizardStore((s) => s.licenseType);
  const licenseNumber = useWizardStore((s) => s.licenseNumber);
  const setField = useWizardStore((s) => s.setField);

  return (
    <Card
      title="What type of license do you hold?"
      description="Regulations differ significantly between centers and family child care homes."
    >
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {OPTIONS.map((opt) => {
          const active = licenseType === opt.value;
          const Icon = opt.icon;
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => setField('licenseType', opt.value)}
              className={`text-left p-4 rounded-lg border transition-colors ${
                active
                  ? 'border-brand-600 bg-brand-50 ring-2 ring-brand-600/20'
                  : 'border-gray-200 hover:border-gray-300'
              }`}
            >
              <Icon className={`h-5 w-5 mb-2 ${active ? 'text-brand-600' : 'text-gray-400'}`} />
              <div className="font-medium text-gray-900">{opt.title}</div>
              <div className="text-xs text-gray-500 mt-0.5">{opt.subtitle}</div>
            </button>
          );
        })}
      </div>

      <div className="mt-5 max-w-md">
        <Input
          label="License number (optional)"
          placeholder="e.g. 191234567"
          hint="If you don't have one yet, skip this and add it later."
          value={licenseNumber}
          onChange={(e) => setField('licenseNumber', e.target.value)}
        />
      </div>

      <div className="mt-6 flex items-center justify-between">
        <Button variant="ghost" onClick={() => navigate('/onboarding/state')}>
          Back
        </Button>
        <Button
          disabled={!licenseType}
          onClick={() => navigate('/onboarding/facility')}
        >
          Continue
        </Button>
      </div>
    </Card>
  );
}
