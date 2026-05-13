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
    first_name: '',
    last_name: '',
    date_of_birth: '',
    enroll_date: '',
    classroom: '',
    allergies: '',
    medical_conditions: '',
    parent_name: '',
    parent_email: '',
    parent_phone: '',
    preferred_contact_channel: 'email' as 'email' | 'sms' | 'both',
  });
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const setField = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr(null);
    if (!form.first_name || !form.last_name || !form.date_of_birth) {
      setErr("Name and date of birth are required to start a child's file.");
      return;
    }
    if (
      (form.preferred_contact_channel === 'sms' ||
        form.preferred_contact_channel === 'both') &&
      !form.parent_phone
    ) {
      setErr('Phone is required when text is the preferred channel.');
      return;
    }
    setBusy(true);
    try {
      const body = {
        ...form,
        date_of_birth: new Date(form.date_of_birth).toISOString(),
        enroll_date: form.enroll_date ? new Date(form.enroll_date).toISOString() : undefined,
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
        className="grid grid-cols-1 sm:grid-cols-2 gap-4"
        noValidate
      >
        <Input
          label="First name"
          required
          value={form.first_name}
          onChange={(e) => setField('first_name', e.target.value)}
        />
        <Input
          label="Last name"
          required
          value={form.last_name}
          onChange={(e) => setField('last_name', e.target.value)}
        />
        <Input
          label="Date of birth"
          type="date"
          required
          value={form.date_of_birth}
          onChange={(e) => setField('date_of_birth', e.target.value)}
        />
        <Input
          label="Enrollment date"
          type="date"
          value={form.enroll_date}
          onChange={(e) => setField('enroll_date', e.target.value)}
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
          value={form.parent_name}
          onChange={(e) => setField('parent_name', e.target.value)}
        />
        <Input
          label="Parent email"
          type="email"
          value={form.parent_email}
          onChange={(e) => setField('parent_email', e.target.value)}
        />
        <Input
          label="Parent phone"
          type="tel"
          value={form.parent_phone}
          onChange={(e) => setField('parent_phone', e.target.value)}
          placeholder="(305) 555-0100"
        />
        <Select
          label="Preferred channel"
          value={form.preferred_contact_channel}
          onChange={(e) =>
            setField(
              'preferred_contact_channel',
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
