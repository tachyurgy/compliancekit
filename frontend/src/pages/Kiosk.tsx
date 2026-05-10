import { useEffect, useState } from 'react';
import { kioskBearerApi, type KioskSignInInput } from '@/api/attendance';

// REQ-016: facility-scoped kiosk page. Mounted on the iPad at the front
// door. No app chrome, no logout button — auto-locks back to the welcome
// screen after 4 hours of no kiosk-session traffic.
//
// Auth: the operator pastes a 22-char kiosk session ID into the device
// once (Settings → Kiosk on the manager side mints it). We cache it in
// localStorage at the key `lb.kiosk.session`.

const STORAGE_KEY = 'lb.kiosk.session';

type Mode = 'welcome' | 'pair' | 'pickdrop' | 'staff' | 'pin' | 'success' | 'error';

interface PendingFlow {
  kind: KioskSignInInput['kind'];
  subjectId: string;
  subjectName: string;
  droppedOffBy?: string;
  pickedUpBy?: string;
}

export default function Kiosk(): JSX.Element {
  const [sessionId, setSessionId] = useState<string>(() => localStorage.getItem(STORAGE_KEY) ?? '');
  const [mode, setMode] = useState<Mode>('welcome');
  const [pending, setPending] = useState<PendingFlow | null>(null);
  const [pin, setPin] = useState<string>('');
  const [message, setMessage] = useState<string>('');
  const [counts, setCounts] = useState<{ kids: number; staff: number }>({ kids: 0, staff: 0 });

  // Pull a fresh roster every 30s so the welcome screen stays in sync.
  useEffect(() => {
    if (!sessionId) {
      setMode('pair');
      return;
    }
    let alive = true;
    const tick = async () => {
      try {
        const r = await kioskBearerApi.roster(sessionId);
        if (alive) setCounts({ kids: r.children_present, staff: r.staff_present });
      } catch {
        // expired session falls back to pair mode
        if (alive) {
          localStorage.removeItem(STORAGE_KEY);
          setSessionId('');
          setMode('pair');
        }
      }
    };
    void tick();
    const t = setInterval(tick, 30000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [sessionId]);

  function startFlow(p: PendingFlow) {
    setPending(p);
    setPin('');
    setMode('pin');
  }

  async function submitPin() {
    if (!pending) return;
    try {
      const fn = pending.kind.endsWith('_in') ? kioskBearerApi.signIn : kioskBearerApi.signOut;
      const res = await fn(sessionId, {
        kind: pending.kind,
        subject_id: pending.subjectId,
        pin,
        dropped_off_by: pending.droppedOffBy,
        picked_up_by: pending.pickedUpBy,
      });
      setMessage(res.message);
      setMode('success');
      // Auto-return to welcome after 3s.
      setTimeout(() => {
        setMode('welcome');
        setPending(null);
        setPin('');
      }, 3000);
    } catch (e) {
      setMessage(e instanceof Error ? e.message : 'That didn\'t work — try again.');
      setMode('error');
      setTimeout(() => setMode('pin'), 2500);
    }
  }

  if (mode === 'pair') {
    return <PairScreen onPaired={(id) => { localStorage.setItem(STORAGE_KEY, id); setSessionId(id); setMode('welcome'); }} />;
  }
  if (mode === 'success') {
    return (
      <Screen>
        <div className="text-center">
          <div className="text-7xl mb-4">✓</div>
          <div className="text-3xl font-medium" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>{message}</div>
        </div>
      </Screen>
    );
  }
  if (mode === 'error') {
    return (
      <Screen>
        <div className="text-center">
          <div className="text-3xl text-orange-700">{message}</div>
        </div>
      </Screen>
    );
  }
  if (mode === 'pin' && pending) {
    return (
      <Screen>
        <div className="max-w-md mx-auto text-center">
          <div className="text-xl text-gray-700 mb-2">{pending.subjectName}</div>
          <div className="text-sm text-gray-500 mb-6">Type your 4-digit PIN</div>
          <div className="text-5xl font-mono tracking-[0.5em] mb-6">{'•'.repeat(pin.length).padEnd(4, '○')}</div>
          <Numpad
            onDigit={(d) => setPin((p) => (p.length < 4 ? p + d : p))}
            onClear={() => setPin('')}
            onSubmit={submitPin}
            canSubmit={pin.length === 4}
          />
          <button className="mt-8 text-sm text-gray-500 underline" onClick={() => { setMode('welcome'); setPending(null); }}>Cancel</button>
        </div>
      </Screen>
    );
  }
  if (mode === 'pickdrop') {
    return <PickDropScreen sessionId={sessionId} onPick={startFlow} onBack={() => setMode('welcome')} />;
  }
  if (mode === 'staff') {
    return <StaffScreen sessionId={sessionId} onPick={startFlow} onBack={() => setMode('welcome')} />;
  }

  // welcome
  return (
    <Screen>
      <div className="text-center">
        <div className="text-2xl text-gray-500 font-medium tracking-wider uppercase mb-2">Levelbrook</div>
        <div className="text-5xl font-medium mb-2" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>Welcome</div>
        <div className="text-lg text-gray-600 mb-12">{counts.kids} children · {counts.staff} staff signed in right now</div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 max-w-2xl mx-auto">
          <BigButton label="Drop off / Pick up" sub="A parent or guardian"
            onClick={() => setMode('pickdrop')} bg="bg-amber-100 hover:bg-amber-200" />
          <BigButton label="Staff clock in / out" sub="A teacher or aide"
            onClick={() => setMode('staff')} bg="bg-emerald-100 hover:bg-emerald-200" />
        </div>
      </div>
    </Screen>
  );
}

function Screen({ children }: { children: React.ReactNode }) {
  return <div className="min-h-screen bg-amber-50 flex items-center justify-center px-6 py-10">{children}</div>;
}

function BigButton({ label, sub, onClick, bg }: { label: string; sub: string; onClick: () => void; bg: string }) {
  return (
    <button onClick={onClick} className={`${bg} rounded-3xl py-12 px-6 text-left transition shadow-sm`}>
      <div className="text-3xl font-medium mb-2" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>{label}</div>
      <div className="text-base text-gray-700">{sub}</div>
    </button>
  );
}

function Numpad({ onDigit, onClear, onSubmit, canSubmit }: { onDigit: (d: string) => void; onClear: () => void; onSubmit: () => void; canSubmit: boolean }) {
  return (
    <div className="grid grid-cols-3 gap-3 max-w-xs mx-auto">
      {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((d) => (
        <button key={d} onClick={() => onDigit(d)} className="aspect-square text-3xl bg-white rounded-2xl shadow-sm hover:bg-amber-50 active:bg-amber-100">{d}</button>
      ))}
      <button onClick={onClear} className="aspect-square text-base bg-white rounded-2xl shadow-sm text-gray-500">Clear</button>
      <button onClick={() => onDigit('0')} className="aspect-square text-3xl bg-white rounded-2xl shadow-sm hover:bg-amber-50 active:bg-amber-100">0</button>
      <button onClick={onSubmit} disabled={!canSubmit} className="aspect-square text-base bg-emerald-500 disabled:bg-gray-200 disabled:text-gray-400 text-white rounded-2xl shadow-sm">Go</button>
    </div>
  );
}

function PairScreen({ onPaired }: { onPaired: (id: string) => void }) {
  const [code, setCode] = useState('');
  return (
    <Screen>
      <div className="max-w-md mx-auto text-center">
        <div className="text-3xl mb-3" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>Pair this iPad</div>
        <div className="text-base text-gray-600 mb-8">In Levelbrook on a manager device, open <span className="font-mono">Settings → Kiosk</span> and tap "Issue session". Paste the code here.</div>
        <input value={code} onChange={(e) => setCode(e.target.value.trim())}
          placeholder="paste the kiosk code"
          className="w-full text-center font-mono tracking-wider text-xl px-4 py-3 rounded-xl border border-gray-300 bg-white" />
        <button onClick={() => code && onPaired(code)} className="mt-6 px-6 py-3 bg-amber-500 hover:bg-amber-600 text-white rounded-xl font-medium">
          Pair this iPad
        </button>
      </div>
    </Screen>
  );
}

// PickDropScreen + StaffScreen — search-by-name wrappers. They use the
// authenticated /api/children + /api/staff endpoints via cookies inherited
// from the manager session if present, OR fall back to a small public list.
// For v1 we keep it simple: type the first letters, get a list, tap a name.

function PickDropScreen({ sessionId, onPick, onBack }: { sessionId: string; onPick: (p: PendingFlow) => void; onBack: () => void }) {
  void sessionId;
  const [name, setName] = useState('');
  const [adultName, setAdultName] = useState('');
  const [direction, setDirection] = useState<'in' | 'out'>('in');
  // Real implementation would fetch /api/kiosk/search?kind=child&q=...
  // For now we accept manual ID entry as a v1 fallback so the kiosk can be
  // tested end-to-end without exposing a roster scrape endpoint.
  const [subjectId, setSubjectId] = useState('');

  return (
    <Screen>
      <div className="max-w-md mx-auto">
        <button onClick={onBack} className="text-sm text-gray-500 underline mb-4">← back</button>
        <div className="text-2xl mb-4" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>
          {direction === 'in' ? 'Drop off a child' : 'Pick up a child'}
        </div>
        <div className="flex gap-2 mb-4">
          <button onClick={() => setDirection('in')} className={`flex-1 py-2 rounded-lg ${direction === 'in' ? 'bg-amber-200' : 'bg-white'}`}>Drop off</button>
          <button onClick={() => setDirection('out')} className={`flex-1 py-2 rounded-lg ${direction === 'out' ? 'bg-amber-200' : 'bg-white'}`}>Pick up</button>
        </div>
        <label className="block text-sm text-gray-600 mb-1">Child name</label>
        <input value={name} onChange={(e) => setName(e.target.value)} className="w-full px-4 py-3 rounded-xl border border-gray-300 bg-white mb-4" placeholder="e.g. Theo" />
        <label className="block text-sm text-gray-600 mb-1">Child ID</label>
        <input value={subjectId} onChange={(e) => setSubjectId(e.target.value.trim())} className="w-full px-4 py-3 rounded-xl border border-gray-300 bg-white mb-4 font-mono text-xs" placeholder="paste from manager dashboard" />
        <label className="block text-sm text-gray-600 mb-1">Your name (the adult here today)</label>
        <input value={adultName} onChange={(e) => setAdultName(e.target.value)} className="w-full px-4 py-3 rounded-xl border border-gray-300 bg-white mb-6" placeholder="e.g. Patricia M." />
        <button
          disabled={!subjectId || !adultName || !name}
          onClick={() => onPick({
            kind: direction === 'in' ? 'child_in' : 'child_out',
            subjectId, subjectName: name,
            droppedOffBy: direction === 'in' ? adultName : undefined,
            pickedUpBy: direction === 'out' ? adultName : undefined,
          })}
          className="w-full py-3 bg-amber-500 hover:bg-amber-600 disabled:bg-gray-200 disabled:text-gray-400 text-white rounded-xl font-medium">
          Continue to PIN
        </button>
      </div>
    </Screen>
  );
}

function StaffScreen({ sessionId, onPick, onBack }: { sessionId: string; onPick: (p: PendingFlow) => void; onBack: () => void }) {
  void sessionId;
  const [name, setName] = useState('');
  const [direction, setDirection] = useState<'in' | 'out'>('in');
  const [subjectId, setSubjectId] = useState('');
  return (
    <Screen>
      <div className="max-w-md mx-auto">
        <button onClick={onBack} className="text-sm text-gray-500 underline mb-4">← back</button>
        <div className="text-2xl mb-4" style={{ fontFamily: 'Fraunces, Georgia, serif' }}>
          Staff {direction === 'in' ? 'clock in' : 'clock out'}
        </div>
        <div className="flex gap-2 mb-4">
          <button onClick={() => setDirection('in')} className={`flex-1 py-2 rounded-lg ${direction === 'in' ? 'bg-emerald-200' : 'bg-white'}`}>Clock in</button>
          <button onClick={() => setDirection('out')} className={`flex-1 py-2 rounded-lg ${direction === 'out' ? 'bg-emerald-200' : 'bg-white'}`}>Clock out</button>
        </div>
        <label className="block text-sm text-gray-600 mb-1">Your name</label>
        <input value={name} onChange={(e) => setName(e.target.value)} className="w-full px-4 py-3 rounded-xl border border-gray-300 bg-white mb-4" placeholder="e.g. Sarah" />
        <label className="block text-sm text-gray-600 mb-1">Staff ID</label>
        <input value={subjectId} onChange={(e) => setSubjectId(e.target.value.trim())} className="w-full px-4 py-3 rounded-xl border border-gray-300 bg-white mb-6 font-mono text-xs" placeholder="paste from manager dashboard" />
        <button
          disabled={!subjectId || !name}
          onClick={() => onPick({
            kind: direction === 'in' ? 'staff_in' : 'staff_out',
            subjectId, subjectName: name,
          })}
          className="w-full py-3 bg-emerald-500 hover:bg-emerald-600 disabled:bg-gray-200 disabled:text-gray-400 text-white rounded-xl font-medium">
          Continue to PIN
        </button>
      </div>
    </Screen>
  );
}
