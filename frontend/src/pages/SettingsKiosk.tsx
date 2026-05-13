import { useState } from 'react';
import PageHeader from '@/components/common/PageHeader';
import Button from '@/components/common/Button';
import { attendanceApi } from '@/api/attendance';

// REQ-016: Settings → Kiosk. Three jobs:
//   1. Issue / display the current 4h kiosk session ID for the iPad pairing.
//   2. Bulk-rotate parent or staff PINs (raw values shown once).
//   3. Future: print the parent welcome letter (deferred to v1.1).

export default function SettingsKiosk(): JSX.Element {
  const [sessionId, setSessionId] = useState<string>('');
  const [expiresAt, setExpiresAt] = useState<string>('');
  const [parentPins, setParentPins] = useState<Record<string, string> | null>(null);
  const [staffPins, setStaffPins] = useState<Record<string, string> | null>(null);
  const [busy, setBusy] = useState<'' | 'session' | 'parents' | 'staff'>('');

  async function issueSession() {
    setBusy('session');
    try {
      const r = await attendanceApi.issueKioskSession('front door iPad');
      setSessionId(r.kiosk_session);
      setExpiresAt(r.expires_at);
    } finally {
      setBusy('');
    }
  }
  async function rotate(scope: 'child' | 'staff') {
    setBusy(scope === 'child' ? 'parents' : 'staff');
    try {
      const r = await attendanceApi.bulkRotatePins(scope);
      if (scope === 'child') setParentPins(r.pins);
      else setStaffPins(r.pins);
    } finally {
      setBusy('');
    }
  }

  return (
    <div>
      <PageHeader
        title="Kiosk"
        description="Pair the iPad at your front door, and manage the 4-digit PINs that parents and staff use to sign in."
      />

      <section className="card p-6 mb-6">
        <h2 className="text-lg font-semibold text-ink-900">Pair an iPad</h2>
        <p className="text-sm text-ink-700 mt-1">
          Open <span className="font-mono">app.levelbrook.com/kiosk</span> on the iPad. Tap "Issue session"
          here, then paste the code on the iPad. The code is good for 4 hours; after that, issue a new one.
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <Button onClick={issueSession} disabled={busy === 'session'}>Issue a kiosk session</Button>
          {sessionId && (
            <div className="flex flex-col">
              <code className="text-base bg-amber-50 px-3 py-2 rounded font-mono">{sessionId}</code>
              <span className="text-xs text-ink-500 mt-1">Expires {new Date(expiresAt).toLocaleString()}</span>
            </div>
          )}
        </div>
      </section>

      <section className="card p-6 mb-6">
        <h2 className="text-lg font-semibold text-ink-900">Parent PINs</h2>
        <p className="text-sm text-ink-700 mt-1">Generate a fresh 4-digit PIN for every enrolled child's parent. Show these once — Levelbrook does not store the digits, only a hash.</p>
        <Button className="mt-4" onClick={() => rotate('child')} disabled={busy === 'parents'}>
          {busy === 'parents' ? 'Rotating…' : 'Generate parent PINs'}
        </Button>
        {parentPins && (
          <div className="mt-4 max-h-64 overflow-auto border border-line-soft rounded-lg p-3 text-sm grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1">
            {Object.entries(parentPins).map(([id, pin]) => (
              <div key={id} className="flex justify-between"><span className="font-mono text-xs text-ink-400">{id.slice(0, 8)}</span><span className="font-mono">{pin}</span></div>
            ))}
          </div>
        )}
      </section>

      <section className="card p-6">
        <h2 className="text-lg font-semibold text-ink-900">Staff PINs</h2>
        <p className="text-sm text-ink-700 mt-1">Generate fresh 4-digit clock-in PINs for everyone on staff.</p>
        <Button className="mt-4" onClick={() => rotate('staff')} disabled={busy === 'staff'}>
          {busy === 'staff' ? 'Rotating…' : 'Generate staff PINs'}
        </Button>
        {staffPins && (
          <div className="mt-4 max-h-64 overflow-auto border border-line-soft rounded-lg p-3 text-sm grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1">
            {Object.entries(staffPins).map(([id, pin]) => (
              <div key={id} className="flex justify-between"><span className="font-mono text-xs text-ink-400">{id.slice(0, 8)}</span><span className="font-mono">{pin}</span></div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
