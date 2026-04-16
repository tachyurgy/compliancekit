import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { CheckCircle2, AlertTriangle } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import { useWizardStore } from '../wizardStore';
import { providersApi } from '@/api/providers';
import { useSession } from '@/hooks/useSession';
import { toast } from '@/components/common/Toast';
import { ApiError } from '@/api/client';

export default function StepReview(): JSX.Element {
  const navigate = useNavigate();
  const wizard = useWizardStore();
  const reset = useWizardStore((s) => s.reset);
  const markOnboardingComplete = useSession((s) => s.markOnboardingComplete);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const missing: string[] = [];
  if (!wizard.stateCode) missing.push('state');
  if (!wizard.licenseType) missing.push('license type');
  if (!wizard.name) missing.push('facility name');
  if (!wizard.address1) missing.push('address');
  if (!wizard.city) missing.push('city');
  if (!wizard.postalCode) missing.push('ZIP');
  if (wizard.capacity == null) missing.push('capacity');

  const handleSubmit = async () => {
    if (missing.length > 0) return;
    setSubmitting(true);
    setError(null);
    try {
      await providersApi.completeOnboarding({
        stateCode: wizard.stateCode!,
        licenseType: wizard.licenseType!,
        licenseNumber: wizard.licenseNumber || undefined,
        name: wizard.name,
        address1: wizard.address1,
        address2: wizard.address2 || undefined,
        city: wizard.city,
        stateRegion: wizard.stateRegion,
        postalCode: wizard.postalCode,
        capacity: wizard.capacity!,
        agesServedMonths: {
          minMonths: wizard.minAgeMonths ?? 0,
          maxMonths: wizard.maxAgeMonths ?? 72,
        },
      });
      markOnboardingComplete();
      reset();
      toast.success('Your compliance checklist is ready.');
      navigate('/dashboard', { replace: true });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Could not save your setup.';
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card
      title="Review and generate your checklist"
      description="We'll build a state-specific compliance checklist from what you've entered."
    >
      {missing.length > 0 && (
        <div className="mb-5 flex items-start gap-3 p-4 rounded-lg bg-critical-50 text-critical-700">
          <AlertTriangle className="h-5 w-5 mt-0.5" />
          <div>
            <div className="font-medium">Missing information</div>
            <div className="text-sm">Please go back and fill in: {missing.join(', ')}.</div>
          </div>
        </div>
      )}

      <dl className="divide-y divide-gray-100 border border-gray-100 rounded-lg">
        <Row label="State" value={wizard.stateCode} />
        <Row
          label="License type"
          value={
            wizard.licenseType === 'center'
              ? 'Child Care Center'
              : wizard.licenseType === 'family_home'
                ? 'Family Child Care Home'
                : null
          }
        />
        <Row label="License number" value={wizard.licenseNumber || '—'} />
        <Row label="Facility name" value={wizard.name} />
        <Row
          label="Address"
          value={
            wizard.address1
              ? `${wizard.address1}${wizard.address2 ? `, ${wizard.address2}` : ''}, ${wizard.city}, ${wizard.stateRegion} ${wizard.postalCode}`
              : null
          }
        />
        <Row label="Capacity" value={wizard.capacity?.toString() ?? null} />
        <Row
          label="Ages served"
          value={
            wizard.minAgeMonths != null && wizard.maxAgeMonths != null
              ? `${wizard.minAgeMonths}–${wizard.maxAgeMonths} months`
              : null
          }
        />
        <Row label="Staff added" value={`${wizard.staff.length}`} />
        <Row label="Children added" value={`${wizard.children.length}`} />
      </dl>

      {error && <p className="mt-4 text-sm text-critical-600">{error}</p>}

      <div className="mt-6 flex items-center justify-between">
        <Button variant="ghost" onClick={() => navigate('/onboarding/children')}>
          Back
        </Button>
        <Button
          onClick={handleSubmit}
          loading={submitting}
          disabled={missing.length > 0}
          leftIcon={<CheckCircle2 className="h-4 w-4" />}
        >
          Generate my checklist
        </Button>
      </div>
    </Card>
  );
}

function Row({ label, value }: { label: string; value: string | null | undefined }): JSX.Element {
  return (
    <div className="grid grid-cols-3 gap-4 px-4 py-3">
      <dt className="text-sm text-gray-500 col-span-1">{label}</dt>
      <dd className="col-span-2 text-sm text-gray-900">
        {value ? value : <span className="text-critical-600">Missing</span>}
      </dd>
    </div>
  );
}
