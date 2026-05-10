/**
 * AddChildModal — REQ-008 add-child form.
 *
 * Mirrors the staff add-modal pattern: full legal name, DOB, room, enrollment
 * date, allergies (free-text), medical conditions, parent contact, and the
 * preferred contact channel (email / sms / both). The channel pick gates
 * whether phone is required. Submit POSTs /api/children directly (the
 * existing useChildren hook isn't shaped for this richer payload yet).
 */
import { useState } from 'react';
import { X } from 'lucide-react';

const API_BASE: string = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export interface AddChildModalProps {
  open: boolean;
  onClose: () => void;
  onCreated?: () => void;
}

export default function AddChildModal({ open, onClose, onCreated }: AddChildModalProps): JSX.Element | null {
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

  if (!open) return null;

  const setField = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const submit = async () => {
    setErr(null);
    if (!form.first_name || !form.last_name || !form.date_of_birth) {
      setErr('Name and date of birth are required.');
      return;
    }
    if ((form.preferred_contact_channel === 'sms' || form.preferred_contact_channel === 'both') && !form.parent_phone) {
      setErr("Phone is required when SMS is the preferred channel.");
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
        throw new Error((j as { error?: { message?: string } })?.error?.message ?? 'Could not save the child.');
      }
      onCreated?.();
      onClose();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Something went wrong on our end.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4">
      <div className="w-full max-w-lg rounded-2xl bg-white shadow-xl">
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-3">
          <h2 className="text-base font-semibold text-gray-900">Add a child</h2>
          <button type="button" onClick={onClose} className="text-gray-400 hover:text-gray-600">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 px-5 py-4">
          <Field label="First name" required>
            <input
              type="text"
              value={form.first_name}
              onChange={(e) => setField('first_name', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Last name" required>
            <input
              type="text"
              value={form.last_name}
              onChange={(e) => setField('last_name', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Date of birth" required>
            <input
              type="date"
              value={form.date_of_birth}
              onChange={(e) => setField('date_of_birth', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Enrollment date">
            <input
              type="date"
              value={form.enroll_date}
              onChange={(e) => setField('enroll_date', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Room">
            <input
              type="text"
              value={form.classroom}
              onChange={(e) => setField('classroom', e.target.value)}
              className="input"
            />
          </Field>
          <div /> {/* grid spacer */}
          <div className="sm:col-span-2">
            <Field label="Allergies">
              <input
                type="text"
                value={form.allergies}
                onChange={(e) => setField('allergies', e.target.value)}
                placeholder="peanuts, eggs"
                className="input"
              />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field label="Medical conditions">
              <input
                type="text"
                value={form.medical_conditions}
                onChange={(e) => setField('medical_conditions', e.target.value)}
                placeholder="asthma, EpiPen"
                className="input"
              />
            </Field>
          </div>

          <div className="sm:col-span-2 mt-3 border-t border-gray-100 pt-3 text-xs uppercase tracking-wider font-medium text-gray-500">
            Parent contact
          </div>
          <Field label="Parent name">
            <input
              type="text"
              value={form.parent_name}
              onChange={(e) => setField('parent_name', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Parent email">
            <input
              type="email"
              value={form.parent_email}
              onChange={(e) => setField('parent_email', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Parent phone">
            <input
              type="tel"
              value={form.parent_phone}
              onChange={(e) => setField('parent_phone', e.target.value)}
              className="input"
            />
          </Field>
          <Field label="Preferred contact">
            <select
              value={form.preferred_contact_channel}
              onChange={(e) => setField('preferred_contact_channel', e.target.value as 'email' | 'sms' | 'both')}
              className="input"
            >
              <option value="email">Email only</option>
              <option value="sms">Text message only</option>
              <option value="both">Both email and text</option>
            </select>
          </Field>
        </div>

        {err && <p className="px-5 pb-2 text-sm text-rose-700">{err}</p>}

        <div className="flex flex-col-reverse gap-2 border-t border-gray-100 px-5 py-3 sm:flex-row sm:justify-end">
          <button type="button" onClick={onClose} className="rounded-xl px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50">
            Cancel
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void submit()}
            className="rounded-xl bg-amber-300 hover:bg-amber-400 px-4 py-2 text-sm font-semibold text-amber-950 disabled:opacity-50"
          >
            {busy ? 'Saving…' : 'Save and continue'}
          </button>
        </div>
      </div>
      <style>{`.input{width:100%;border:1px solid rgb(229,231,235);background:white;border-radius:.75rem;padding:.6rem .75rem;font-size:14px}.input:focus{outline:none;border-color:rgb(245,158,11);box-shadow:0 0 0 3px rgba(252,211,77,.4)}`}</style>
    </div>
  );
}

function Field({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }): JSX.Element {
  return (
    <label className="block text-sm">
      <span className="text-gray-700">
        {label}
        {required && <span className="text-rose-600">*</span>}
      </span>
      <div className="mt-1">{children}</div>
    </label>
  );
}
