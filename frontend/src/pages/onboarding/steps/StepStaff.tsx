import { useNavigate } from 'react-router-dom';
import { useState } from 'react';
import { Plus, Trash2, Upload } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import Select from '@/components/common/Select';
import EmptyState from '@/components/common/EmptyState';
import { useWizardStore, type StaffDraft } from '../wizardStore';

const ROLE_OPTIONS = [
  { value: 'director', label: 'Director' },
  { value: 'lead_teacher', label: 'Lead Teacher' },
  { value: 'assistant', label: 'Assistant' },
  { value: 'aide', label: 'Aide' },
  { value: 'cook', label: 'Cook' },
  { value: 'other', label: 'Other' },
];

export default function StepStaff(): JSX.Element {
  const navigate = useNavigate();
  const staff = useWizardStore((s) => s.staff);
  const addStaff = useWizardStore((s) => s.addStaff);
  const removeStaff = useWizardStore((s) => s.removeStaff);

  const [draft, setDraft] = useState<StaffDraft>({
    firstName: '',
    lastName: '',
    email: '',
    role: 'lead_teacher',
  });

  const canAdd = draft.firstName.trim() && draft.lastName.trim();

  const handleAdd = () => {
    if (!canAdd) return;
    addStaff({
      firstName: draft.firstName.trim(),
      lastName: draft.lastName.trim(),
      email: draft.email?.trim() || undefined,
      role: draft.role,
    });
    setDraft({ firstName: '', lastName: '', email: '', role: 'lead_teacher' });
  };

  const handleCsvUpload = async (file: File) => {
    const text = await file.text();
    const rows = text
      .split(/\r?\n/)
      .map((row) => row.trim())
      .filter(Boolean);
    // skip header if present
    const startIdx = /first/i.test(rows[0] ?? '') ? 1 : 0;
    for (let i = startIdx; i < rows.length; i++) {
      const parts = rows[i].split(',').map((p) => p.trim());
      const [firstName, lastName, email, role] = parts;
      if (!firstName || !lastName) continue;
      const normalizedRole = (ROLE_OPTIONS.find((o) => o.value === role)?.value ??
        'lead_teacher') as StaffDraft['role'];
      addStaff({
        firstName,
        lastName,
        email: email || undefined,
        role: normalizedRole,
      });
    }
  };

  return (
    <Card
      title="Who's on your team?"
      description="Add your current staff. We'll pull in their certifications in the next step. You can also skip this and add them later."
    >
      <div className="grid grid-cols-1 md:grid-cols-[1fr_1fr_1fr_auto] gap-2 items-end">
        <Input
          label="First name"
          value={draft.firstName}
          onChange={(e) => setDraft({ ...draft, firstName: e.target.value })}
        />
        <Input
          label="Last name"
          value={draft.lastName}
          onChange={(e) => setDraft({ ...draft, lastName: e.target.value })}
        />
        <Select
          label="Role"
          value={draft.role}
          options={ROLE_OPTIONS}
          onChange={(e) =>
            setDraft({ ...draft, role: e.target.value as StaffDraft['role'] })
          }
        />
        <Button
          type="button"
          onClick={handleAdd}
          disabled={!canAdd}
          leftIcon={<Plus className="h-4 w-4" />}
        >
          Add
        </Button>
      </div>
      <div className="mt-2">
        <Input
          label="Email (optional)"
          type="email"
          value={draft.email ?? ''}
          onChange={(e) => setDraft({ ...draft, email: e.target.value })}
          hint="We'll invite them to upload their own certifications."
        />
      </div>

      <div className="mt-4">
        <label className="inline-flex items-center gap-2 text-sm font-medium text-gray-700 cursor-pointer">
          <Upload className="h-4 w-4" />
          <span className="underline underline-offset-2">Or import from CSV</span>
          <input
            type="file"
            accept=".csv,text/csv"
            className="sr-only"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) void handleCsvUpload(file);
              e.target.value = '';
            }}
          />
        </label>
        <p className="mt-1 text-xs text-gray-500">
          Expected columns: <code>first_name, last_name, email, role</code>
        </p>
      </div>

      <div className="mt-6">
        {staff.length === 0 ? (
          <EmptyState
            title="No staff added yet"
            description="You can still continue — staff can be added anytime from the dashboard."
          />
        ) : (
          <ul className="divide-y divide-gray-100 border border-gray-100 rounded-lg bg-white">
            {staff.map((member, idx) => (
              <li
                key={`${member.firstName}-${idx}`}
                className="flex items-center justify-between px-4 py-3"
              >
                <div>
                  <div className="text-sm font-medium text-gray-900">
                    {member.firstName} {member.lastName}
                  </div>
                  <div className="text-xs text-gray-500">
                    {ROLE_OPTIONS.find((o) => o.value === member.role)?.label}
                    {member.email && <span> &middot; {member.email}</span>}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => removeStaff(idx)}
                  aria-label="Remove staff member"
                  className="p-1 text-gray-400 hover:text-critical-600"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="mt-6 flex items-center justify-between">
        <Button variant="ghost" onClick={() => navigate('/onboarding/facility')}>
          Back
        </Button>
        <Button onClick={() => navigate('/onboarding/children')}>Continue</Button>
      </div>
    </Card>
  );
}
