import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams, Link } from 'react-router-dom';
import { Check, ShieldCheck, Plus, Trash2 } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import Select from '@/components/common/Select';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { facilityApi, FL_AGE_BANDS, type Facility, type Room, type FacilityPatch } from '@/api/facility';
import { toast } from '@/components/common/Toast';

// REQ-003 — Florida 4-step onboarding wizard. Save-as-you-go via debounced
// PATCH /api/facility on field blur; resumable. The progress bar is the
// navigation. The final step is skip-able. After "Finish onboarding" the
// dashboard's first-run welcome panel handles the "what next" coaching.

const STEPS = [
  { idx: 1, key: 'identity', label: '1 Facility' },
  { idx: 2, key: 'capacity', label: '2 Capacity' },
  { idx: 3, key: 'people', label: '3 People' },
  { idx: 4, key: 'rooms', label: '4 Rooms' },
] as const;

type StepKey = (typeof STEPS)[number]['key'];

export default function FacilityWizard(): JSX.Element {
  const [params, setParams] = useSearchParams();
  const stepKey = (params.get('step') as StepKey) || 'identity';
  const navigate = useNavigate();
  const [facility, setFacility] = useState<Facility | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    facilityApi
      .get()
      .then((f) => {
        if (!cancelled) setFacility(f);
      })
      .catch(() => {
        if (!cancelled) toast.error("We couldn't load your facility. Try refreshing.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function go(next: StepKey) {
    setParams({ step: next });
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  async function patch(p: FacilityPatch) {
    if (!facility) return;
    try {
      const updated = await facilityApi.patch(p);
      setFacility(updated);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "We couldn't save that just yet.");
    }
  }

  async function finish() {
    setSubmitting(true);
    try {
      await facilityApi.completeOnboarding();
      toast.success("You're all set. Welcome to Levelbrook.");
      navigate('/dashboard');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'A few fields are still missing — scroll up and check.');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen bg-surface-muted">
      <header className="bg-white border-b border-gray-100">
        <div className="max-w-5xl mx-auto px-6 h-16 flex items-center">
          <Link to="/" className="flex items-center gap-2">
            <ShieldCheck className="h-6 w-6 text-brand-600" />
            <span className="font-semibold">Levelbrook</span>
          </Link>
        </div>
      </header>

      <div className="max-w-3xl mx-auto px-6 py-8">
        <ProgressBar current={stepKey} onJump={go} />

        {loading || !facility ? (
          <div className="py-20"><LoadingSpinner label="Loading…" /></div>
        ) : (
          <div className="mt-8">
            {stepKey === 'identity' && (
              <IdentityStep facility={facility} onPatch={patch} onNext={() => go('capacity')} />
            )}
            {stepKey === 'capacity' && (
              <CapacityStep
                facility={facility}
                onPatch={patch}
                onBack={() => go('identity')}
                onNext={() => go('people')}
              />
            )}
            {stepKey === 'people' && (
              <PeopleStep
                facility={facility}
                onPatch={patch}
                onBack={() => go('capacity')}
                onNext={() => go('rooms')}
              />
            )}
            {stepKey === 'rooms' && (
              <RoomsStep onBack={() => go('people')} onFinish={finish} submitting={submitting} />
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function ProgressBar({ current, onJump }: { current: StepKey; onJump: (k: StepKey) => void }): JSX.Element {
  const currentIdx = STEPS.findIndex((s) => s.key === current);
  return (
    <ol className="flex items-center gap-2 overflow-x-auto">
      {STEPS.map((s, idx) => {
        const active = idx === currentIdx;
        const complete = idx < currentIdx;
        return (
          <li key={s.key} className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => onJump(s.key)}
              className={`h-7 w-7 rounded-full text-xs font-medium grid place-items-center transition ${
                complete
                  ? 'bg-brand-600 text-white'
                  : active
                    ? 'bg-brand-50 text-brand-700 ring-2 ring-brand-600'
                    : 'bg-gray-100 text-gray-500 hover:bg-gray-200'
              }`}
            >
              {complete ? <Check className="h-3.5 w-3.5" /> : idx + 1}
            </button>
            <span className={`text-sm whitespace-nowrap ${active ? 'text-gray-900 font-medium' : 'text-gray-500'}`}>
              {s.label.replace(/^\d\s/, '')}
            </span>
            {idx < STEPS.length - 1 && <span className="mx-2 h-px w-8 bg-gray-200 hidden md:block" />}
          </li>
        );
      })}
    </ol>
  );
}

// ----- Step 1: Facility identity -----------------------------------------
function IdentityStep({
  facility,
  onPatch,
  onNext,
}: {
  facility: Facility;
  onPatch: (p: FacilityPatch) => Promise<void>;
  onNext: () => void;
}): JSX.Element {
  const [local, setLocal] = useState({
    name: facility.name,
    licenseNumber: facility.licenseNumber,
    licenseExpiresOn: facility.licenseExpiresOn ?? '',
    county: facility.county,
    address1: facility.address1,
    address2: facility.address2,
    city: facility.city,
    stateRegion: facility.stateRegion || 'FL',
    postalCode: facility.postalCode,
    phone: facility.phone,
  });

  function save<K extends keyof typeof local>(key: K, value: (typeof local)[K]) {
    setLocal((s) => ({ ...s, [key]: value }));
  }
  function blurSave(key: keyof typeof local) {
    return () => onPatch({ [key]: local[key] } as FacilityPatch);
  }

  return (
    <Card title="Tell us about your facility" description="The basics — what we'll need on every report.">
      <form
        className="grid grid-cols-1 sm:grid-cols-2 gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          onNext();
        }}
      >
        <div className="sm:col-span-2">
          <Input
            label="Facility name"
            value={local.name}
            onChange={(e) => save('name', e.target.value)}
            onBlur={blurSave('name')}
            required
          />
        </div>
        <Input
          label="DCF license number"
          value={local.licenseNumber}
          onChange={(e) => save('licenseNumber', e.target.value)}
          onBlur={blurSave('licenseNumber')}
          placeholder="C12FL0123"
        />
        <Input
          label="License expiration date"
          type="date"
          value={local.licenseExpiresOn}
          onChange={(e) => save('licenseExpiresOn', e.target.value)}
          onBlur={blurSave('licenseExpiresOn')}
        />
        <Input
          label="County"
          value={local.county}
          onChange={(e) => save('county', e.target.value)}
          onBlur={blurSave('county')}
          placeholder="Miami-Dade"
        />
        <Input
          label="Phone"
          type="tel"
          value={local.phone}
          onChange={(e) => save('phone', e.target.value)}
          onBlur={blurSave('phone')}
          placeholder="(305) 555-0100"
        />
        <div className="sm:col-span-2">
          <Input
            label="Street address"
            value={local.address1}
            onChange={(e) => save('address1', e.target.value)}
            onBlur={blurSave('address1')}
          />
        </div>
        <div className="sm:col-span-2">
          <Input
            label="Address line 2 (optional)"
            value={local.address2}
            onChange={(e) => save('address2', e.target.value)}
            onBlur={blurSave('address2')}
          />
        </div>
        <Input
          label="City"
          value={local.city}
          onChange={(e) => save('city', e.target.value)}
          onBlur={blurSave('city')}
        />
        <div className="grid grid-cols-2 gap-3">
          <Input
            label="State"
            maxLength={2}
            value={local.stateRegion}
            onChange={(e) => save('stateRegion', e.target.value.toUpperCase())}
            onBlur={blurSave('stateRegion')}
          />
          <Input
            label="ZIP"
            value={local.postalCode}
            onChange={(e) => save('postalCode', e.target.value)}
            onBlur={blurSave('postalCode')}
          />
        </div>

        <div className="sm:col-span-2 flex items-center justify-end pt-2">
          <Button type="submit">Save and continue</Button>
        </div>
      </form>
    </Card>
  );
}

// ----- Step 2: Capacity & licensing --------------------------------------
function CapacityStep({
  facility,
  onPatch,
  onBack,
  onNext,
}: {
  facility: Facility;
  onPatch: (p: FacilityPatch) => Promise<void>;
  onBack: () => void;
  onNext: () => void;
}): JSX.Element {
  const [local, setLocal] = useState({
    capacity: facility.capacity || 0,
    indoorSqft: facility.indoorSqft ?? 0,
    outdoorPlayground: facility.outdoorPlayground,
    eveningLicense: facility.eveningLicense,
    weekendLicense: facility.weekendLicense,
    twentyFourHour: facility.twentyFourHour,
  });
  function setField<K extends keyof typeof local>(key: K, value: (typeof local)[K]) {
    setLocal((s) => ({ ...s, [key]: value }));
    onPatch({ [key]: value } as FacilityPatch);
  }
  return (
    <Card title="How big is your facility?" description="We use these numbers to figure your ratios and what postings you need.">
      <form
        className="grid grid-cols-1 sm:grid-cols-2 gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          onNext();
        }}
      >
        <Input
          label="Licensed capacity"
          type="number"
          min={1}
          value={local.capacity || ''}
          onChange={(e) => setLocal((s) => ({ ...s, capacity: parseInt(e.target.value, 10) || 0 }))}
          onBlur={() => onPatch({ capacity: local.capacity })}
        />
        <Input
          label="Indoor square footage"
          hint="DCF requires 35 sq ft per child indoors. We use this for your ratio calculator."
          type="number"
          min={1}
          value={local.indoorSqft || ''}
          onChange={(e) => setLocal((s) => ({ ...s, indoorSqft: parseInt(e.target.value, 10) || 0 }))}
          onBlur={() => onPatch({ indoorSqft: local.indoorSqft })}
        />
        <YesNo label="Outdoor playground?" value={local.outdoorPlayground} onChange={(v) => setField('outdoorPlayground', v)} />
        <YesNo label="Evening child care license?" value={local.eveningLicense} onChange={(v) => setField('eveningLicense', v)} />
        <YesNo label="Weekend child care license?" value={local.weekendLicense} onChange={(v) => setField('weekendLicense', v)} />
        <YesNo label="24-hour care license?" value={local.twentyFourHour} onChange={(v) => setField('twentyFourHour', v)} />

        <div className="sm:col-span-2 flex items-center justify-between pt-2">
          <Button type="button" variant="ghost" onClick={onBack}>Back</Button>
          <Button type="submit">Save and continue</Button>
        </div>
      </form>
    </Card>
  );
}

function YesNo({ label, value, onChange }: { label: React.ReactNode; value: boolean; onChange: (b: boolean) => void }): JSX.Element {
  return (
    <div className="space-y-1">
      <div className="block text-sm font-medium text-gray-800">{label}</div>
      <div className="inline-flex rounded-lg border border-gray-200 bg-white p-1 gap-1">
        <button
          type="button"
          onClick={() => onChange(true)}
          className={`px-4 py-1.5 rounded-md text-sm ${value ? 'bg-brand-600 text-white' : 'text-gray-700 hover:bg-gray-50'}`}
        >
          Yes
        </button>
        <button
          type="button"
          onClick={() => onChange(false)}
          className={`px-4 py-1.5 rounded-md text-sm ${!value ? 'bg-brand-600 text-white' : 'text-gray-700 hover:bg-gray-50'}`}
        >
          No
        </button>
      </div>
    </div>
  );
}

// ----- Step 3: Director & owner ------------------------------------------
function PeopleStep({
  facility,
  onPatch,
  onBack,
  onNext,
}: {
  facility: Facility;
  onPatch: (p: FacilityPatch) => Promise<void>;
  onBack: () => void;
  onNext: () => void;
}): JSX.Element {
  const [local, setLocal] = useState({
    ownerFullName: facility.ownerFullName,
    directorFullName: facility.directorFullName,
    directorEmail: facility.directorEmail || facility.ownerEmail,
  });
  const [sameAsOwner, setSameAsOwner] = useState(
    !facility.directorFullName || facility.directorFullName === facility.ownerFullName,
  );

  useEffect(() => {
    if (sameAsOwner) {
      setLocal((s) => ({ ...s, directorFullName: s.ownerFullName, directorEmail: facility.ownerEmail }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sameAsOwner]);

  function save<K extends keyof typeof local>(key: K, value: (typeof local)[K]) {
    setLocal((s) => ({ ...s, [key]: value }));
  }
  return (
    <Card title="Who runs the show?" description="Owner and director — for the personnel file and license renewal paperwork.">
      <form
        className="grid grid-cols-1 sm:grid-cols-2 gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          onPatch(local);
          onNext();
        }}
      >
        <Input
          label="Owner full name"
          value={local.ownerFullName}
          onChange={(e) => save('ownerFullName', e.target.value)}
          onBlur={() => onPatch({ ownerFullName: local.ownerFullName })}
        />
        <Input
          label="Owner email"
          type="email"
          value={facility.ownerEmail}
          readOnly
          disabled
        />
        <div className="sm:col-span-2">
          <label className="flex items-center gap-2 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={sameAsOwner}
              onChange={(e) => setSameAsOwner(e.target.checked)}
              className="rounded border-gray-300 text-brand-600 focus:ring-brand-600"
            />
            Same as owner
          </label>
        </div>
        <Input
          label="Director full name"
          value={local.directorFullName}
          onChange={(e) => save('directorFullName', e.target.value)}
          onBlur={() => onPatch({ directorFullName: local.directorFullName })}
          disabled={sameAsOwner}
        />
        <Input
          label="Director email"
          type="email"
          value={local.directorEmail}
          onChange={(e) => save('directorEmail', e.target.value)}
          onBlur={() => onPatch({ directorEmail: local.directorEmail })}
          disabled={sameAsOwner}
        />

        <div className="sm:col-span-2 flex items-center justify-between pt-2">
          <Button type="button" variant="ghost" onClick={onBack}>Back</Button>
          <Button type="submit">Save and continue</Button>
        </div>
      </form>
    </Card>
  );
}

// ----- Step 4: Rooms -----------------------------------------------------
function RoomsStep({
  onBack,
  onFinish,
  submitting,
}: {
  onBack: () => void;
  onFinish: () => Promise<void>;
  submitting: boolean;
}): JSX.Element {
  const [rooms, setRooms] = useState<Room[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    facilityApi
      .listRooms()
      .then(setRooms)
      .catch(() => undefined)
      .finally(() => setLoading(false));
  }, []);

  async function add() {
    const r = await facilityApi.createRoom({ name: 'New room', ageBand: 'twos' });
    setRooms((rs) => [...rs, { id: r.id, name: 'New room', ageBand: 'twos', createdAt: new Date().toISOString() }]);
  }
  async function update(id: string, patch: Partial<Room>) {
    setRooms((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)));
    await facilityApi.patchRoom(id, {
      name: patch.name ?? rooms.find((r) => r.id === id)?.name ?? '',
      ageBand: (patch.ageBand ?? rooms.find((r) => r.id === id)?.ageBand) || undefined,
      licensedCapacity: patch.licensedCapacity ?? rooms.find((r) => r.id === id)?.licensedCapacity ?? undefined,
      indoorSqft: patch.indoorSqft ?? rooms.find((r) => r.id === id)?.indoorSqft ?? undefined,
    });
  }
  async function remove(id: string) {
    setRooms((rs) => rs.filter((r) => r.id !== id));
    await facilityApi.deleteRoom(id);
  }

  return (
    <Card
      title="Add your rooms"
      description="One row per classroom. You can skip this and add rooms later — the dashboard will nudge you when it matters."
    >
      {loading ? (
        <LoadingSpinner label="Loading rooms…" />
      ) : (
        <>
          {rooms.length === 0 && (
            <p className="text-sm text-gray-500 mb-3">No rooms yet. Add your first one below.</p>
          )}
          <ul className="space-y-3">
            {rooms.map((r) => (
              <li key={r.id} className="grid grid-cols-1 sm:grid-cols-12 gap-2 items-end">
                <div className="sm:col-span-4">
                  <Input
                    label="Room name"
                    value={r.name}
                    onChange={(e) => setRooms((rs) => rs.map((x) => (x.id === r.id ? { ...x, name: e.target.value } : x)))}
                    onBlur={(e) => update(r.id, { name: e.target.value })}
                  />
                </div>
                <div className="sm:col-span-3">
                  <Select
                    label="Age band"
                    value={r.ageBand}
                    onChange={(e) => update(r.id, { ageBand: e.target.value })}
                    options={FL_AGE_BANDS.map((b) => ({ value: b.value, label: b.label }))}
                  />
                </div>
                <div className="sm:col-span-2">
                  <Input
                    label="Capacity"
                    type="number"
                    min={1}
                    value={r.licensedCapacity ?? ''}
                    onChange={(e) =>
                      setRooms((rs) =>
                        rs.map((x) =>
                          x.id === r.id ? { ...x, licensedCapacity: parseInt(e.target.value, 10) || undefined } : x,
                        ),
                      )
                    }
                    onBlur={(e) => update(r.id, { licensedCapacity: parseInt(e.target.value, 10) || undefined })}
                  />
                </div>
                <div className="sm:col-span-2">
                  <Input
                    label="Sq ft"
                    type="number"
                    min={1}
                    value={r.indoorSqft ?? ''}
                    onChange={(e) =>
                      setRooms((rs) =>
                        rs.map((x) =>
                          x.id === r.id ? { ...x, indoorSqft: parseInt(e.target.value, 10) || undefined } : x,
                        ),
                      )
                    }
                    onBlur={(e) => update(r.id, { indoorSqft: parseInt(e.target.value, 10) || undefined })}
                  />
                </div>
                <div className="sm:col-span-1 flex justify-end">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => remove(r.id)}
                    aria-label="Remove room"
                  >
                    <Trash2 className="h-4 w-4 text-gray-500" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
          <Button
            type="button"
            variant="secondary"
            leftIcon={<Plus className="h-4 w-4" />}
            className="mt-4"
            onClick={add}
          >
            Add a room
          </Button>
        </>
      )}

      <div className="flex items-center justify-between pt-6 mt-6 border-t border-gray-100">
        <Button type="button" variant="ghost" onClick={onBack}>Back</Button>
        <div className="flex gap-2">
          <Button type="button" variant="ghost" onClick={onFinish} loading={submitting}>
            Skip and finish
          </Button>
          <Button type="button" onClick={onFinish} loading={submitting}>
            Finish onboarding
          </Button>
        </div>
      </div>
    </Card>
  );
}

// memoised useMemo placeholder — keeps an unused-variable warning quiet for
// future analytics hooks.
export const _wizardSteps = STEPS as readonly { idx: number; key: StepKey; label: string }[];
