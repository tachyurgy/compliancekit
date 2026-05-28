/**
 * AddChildModal — REQ-008 / REQ-012d add-child form.
 *
 * Mirrors AddStaffModal: full legal name, DOB, room, enrollment date,
 * allergies, medical conditions, parent contact + preferred channel. The
 * channel pick gates whether phone is required. Submit POSTs /api/children
 * directly (the existing useChildren hook isn't shaped for this richer
 * payload yet).
 */
import { useState } from 'react';

import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import Select from '@/components/common/Select';
import Modal from '@/components/common/Modal';

const API_BASE: string = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export interface AddChildModalProps {
  open: boolean;
  onClose: () => void;
  onCreated?: () => void;
}

export default function AddChildModal({
  open,
  onClose,
  onCreated,
}: AddChildModalProps): JSX.Element {
  const [form, setForm] = useState({
    firstName: '',
    lastName: '',
    dateOfBirth: '',
    enrollmentDate: '',
    classroom: '',
    allergies: '',
    medical_conditions: '',
    parentName: '',
    parentEmail: '',
    parentPhone: '',
    preferredContactChannel: 'email' as 'email' | 'sms' | 'both',
  });
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const setField = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr(null);
    if (!form.firstName || !form.lastName || !form.dateOfBirth) {
      setErr("Name and date of birth are required to start a child's file.");
      return;
    }
    if (
      (form.preferredContactChannel === 'sms' ||
        form.preferredContactChannel === 'both') &&
      !form.parentPhone
    ) {
      setErr('Phone is required when text is the preferred channel.');
      return;
    }
    setBusy(true);
    try {
      const body = {
        ...form,
        dateOfBirth: new Date(form.dateOfBirth).toISOString(),
        enrollmentDate: form.enrollmentDate ? new Date(form.enrollmentDate).toISOString() : undefined,
      };
      const r = await fetch(`${API_BASE}/api/children`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!r.ok) {
        const j = await r.json().catch(() => null);
        throw new Error(
          (j as { error?: { message?: string } })?.error?.message ?? 'Could not save the child.',
        );
      }
      onCreated?.();
      onClose();
    } catch (e) {
      setErr(
        e instanceof Error
          ? e.message
          : 'Something went wrong on our end. Please try again in a minute.',
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="lg"
      title="Add a child"
      description="We'll start tracking their file and chase the parents for any missing forms."
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button form="add-child-form" type="submit" loading={busy}>
            Save and continue
          </Button>
        </>
      }
    >
      <form
        id="add-child-form"
        onSubmit={submit}
        className="grid grid-cols-1 sm:grid-cols-2 gap-4 min-w-0 max-w-full [&>*]:min-w-0"
        noValidate
      >
        <Input
          label="First name"
          required
          value={form.firstName}
          onChange={(e) => setField('firstName', e.target.value)}
        />
        <Input
          label="Last name"
          required
          value={form.lastName}
          onChange={(e) => setField('lastName', e.target.value)}
        />
        <Input
          label="Date of birth"
          type="date"
          required
          value={form.dateOfBirth}
          onChange={(e) => setField('dateOfBirth', e.target.value)}
        />
        <Input
          label="Enrollment date"
          type="date"
          value={form.enrollmentDate}
          onChange={(e) => setField('enrollmentDate', e.target.value)}
        />
        <Input
          label="Room"
          value={form.classroom}
          onChange={(e) => setField('classroom', e.target.value)}
        />
        <div className="hidden sm:block" />
        <div className="sm:col-span-2">
          <Input
            label="Allergies"
            value={form.allergies}
            onChange={(e) => setField('allergies', e.target.value)}
            placeholder="peanuts, eggs"
          />
        </div>
        <div className="sm:col-span-2">
          <Input
            label="Medical conditions"
            value={form.medical_conditions}
            onChange={(e) => setField('medical_conditions', e.target.value)}
            placeholder="asthma, EpiPen"
          />
        </div>

        <div className="sm:col-span-2 mt-3 pt-4 border-t border-line-soft">
          <div className="eyebrow">Parent contact</div>
        </div>
        <Input
          label="Parent name"
          value={form.parentName}
          onChange={(e) => setField('parentName', e.target.value)}
        />
        <Input
          label="Parent email"
          type="email"
          value={form.parentEmail}
          onChange={(e) => setField('parentEmail', e.target.value)}
        />
        <Input
          label="Parent phone"
          type="tel"
          value={form.parentPhone}
          onChange={(e) => setField('parentPhone', e.target.value)}
          placeholder="(305) 555-0100"
        />
        <Select
          label="Preferred channel"
          value={form.preferredContactChannel}
          onChange={(e) =>
            setField(
              'preferredContactChannel',
              e.target.value as 'email' | 'sms' | 'both',
            )
          }
          options={[
            { value: 'email', label: 'Email only' },
            { value: 'sms', label: 'Text only' },
            { value: 'both', label: 'Email + text' },
          ]}
        />

        {err && (
          <div
            className="sm:col-span-2 rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
            role="alert"
          >
            {err}
          </div>
        )}
      </form>
    </Modal>
  );
}
