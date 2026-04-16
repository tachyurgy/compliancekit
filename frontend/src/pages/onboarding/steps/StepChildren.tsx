import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Plus, Trash2, Upload } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import EmptyState from '@/components/common/EmptyState';
import { useWizardStore, type ChildDraft } from '../wizardStore';

export default function StepChildren(): JSX.Element {
  const navigate = useNavigate();
  const children = useWizardStore((s) => s.children);
  const addChild = useWizardStore((s) => s.addChild);
  const removeChild = useWizardStore((s) => s.removeChild);

  const [draft, setDraft] = useState<ChildDraft>({
    firstName: '',
    lastName: '',
    dateOfBirth: '',
    parentEmail: '',
  });

  const canAdd =
    draft.firstName.trim() && draft.lastName.trim() && /^\d{4}-\d{2}-\d{2}$/.test(draft.dateOfBirth);

  const handleAdd = () => {
    if (!canAdd) return;
    addChild({
      firstName: draft.firstName.trim(),
      lastName: draft.lastName.trim(),
      dateOfBirth: draft.dateOfBirth,
      parentEmail: draft.parentEmail?.trim() || undefined,
    });
    setDraft({ firstName: '', lastName: '', dateOfBirth: '', parentEmail: '' });
  };

  const handleCsvUpload = async (file: File) => {
    const text = await file.text();
    const rows = text.split(/\r?\n/).map((r) => r.trim()).filter(Boolean);
    const startIdx = /first/i.test(rows[0] ?? '') ? 1 : 0;
    for (let i = startIdx; i < rows.length; i++) {
      const [firstName, lastName, dateOfBirth, parentEmail] = rows[i].split(',').map((p) => p.trim());
      if (!firstName || !lastName || !dateOfBirth) continue;
      if (!/^\d{4}-\d{2}-\d{2}$/.test(dateOfBirth)) continue;
      addChild({ firstName, lastName, dateOfBirth, parentEmail: parentEmail || undefined });
    }
  };

  return (
    <Card
      title="Who attends your program?"
      description="Add your current children. We'll build their required-doc checklist based on your state's rules."
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
        <Input
          label="Date of birth"
          type="date"
          value={draft.dateOfBirth}
          onChange={(e) => setDraft({ ...draft, dateOfBirth: e.target.value })}
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
          label="Parent email (optional)"
          type="email"
          value={draft.parentEmail ?? ''}
          onChange={(e) => setDraft({ ...draft, parentEmail: e.target.value })}
          hint="We'll email the parent a portal link to upload immunization records and forms."
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
          Expected columns: <code>first_name, last_name, date_of_birth (YYYY-MM-DD), parent_email</code>
        </p>
      </div>

      <div className="mt-6">
        {children.length === 0 ? (
          <EmptyState
            title="No children added yet"
            description="You can still continue — children can be added anytime from the dashboard."
          />
        ) : (
          <ul className="divide-y divide-gray-100 border border-gray-100 rounded-lg bg-white">
            {children.map((child, idx) => (
              <li
                key={`${child.firstName}-${idx}`}
                className="flex items-center justify-between px-4 py-3"
              >
                <div>
                  <div className="text-sm font-medium text-gray-900">
                    {child.firstName} {child.lastName}
                  </div>
                  <div className="text-xs text-gray-500">
                    DOB {child.dateOfBirth}
                    {child.parentEmail && <span> &middot; {child.parentEmail}</span>}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => removeChild(idx)}
                  aria-label="Remove child"
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
        <Button variant="ghost" onClick={() => navigate('/onboarding/staff')}>
          Back
        </Button>
        <Button onClick={() => navigate('/onboarding/review')}>Continue</Button>
      </div>
    </Card>
  );
}
