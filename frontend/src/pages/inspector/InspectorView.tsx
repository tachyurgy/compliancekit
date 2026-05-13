/**
 * Inspector Mode (REQ-009).
 *
 * iPad-landscape primary surface: two columns (left tab list, right
 * content). The whole page is themed against the navy/parchment palette
 * defined in tailwind.config.ts under `inspector.*` — we explicitly do
 * NOT inherit the operator-side warm-yellow palette here. Inspector
 * Mode is a separate visual identity by design.
 *
 * Form-name fidelity rule (overrides brand-voice.md plain-English):
 *   - Show "CF-FSP 5316", "DH 680", "Handbook § 7.4" verbatim.
 *   - Show real status pills ("On file", "Expired", "Missing", "Expiring soon").
 *   - Empty states are honest: "No drills logged in the last 12 months".
 */

import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import {
  addFinding,
  endSession,
  getSnapshot,
  InspectorSnapshot,
  PersonnelItem,
  pullSample,
  StaffCard,
  ChildCard,
  startSession,
  StatusPill,
  planOfCorrectionURL,
  Finding,
} from '@/api/inspector';

type TabKey =
  | 'snapshot'
  | 'personnel'
  | 'children'
  | 'records'
  | 'postings'
  | 'drills'
  | 'exit';

const TABS: { key: TabKey; label: string; subtitle: string }[] = [
  { key: 'snapshot', label: '1. Snapshot', subtitle: 'Facility identity' },
  { key: 'personnel', label: '2. Personnel sample', subtitle: 'Handbook § 7.4' },
  { key: 'children', label: '3. Children sample', subtitle: 'Handbook § 7.3' },
  { key: 'records', label: '4. Facility records', subtitle: 'Handbook § 7' },
  { key: 'postings', label: '5. Postings', subtitle: 'Handbook § 19' },
  { key: 'drills', label: '6. Drills + walkthroughs', subtitle: 'Last 12 months' },
  { key: 'exit', label: '7. Exit conference', subtitle: 'Plan of Correction' },
];

// CF-FSP 5316 most-common findings dictionary. Each entry maps to a 5316
// line number, a violation class, and the canonical FL violation language.
// Keep this short; the inspector wants 10 of these on tap, not 100.
const STANDARD_FINDINGS: { line: string; class: string; text: string }[] = [
  {
    line: '5316 #11',
    class: 'Class II',
    text:
      'Background screening eligible-result document not on file (Handbook § 5.1; F.S. § 435.04).',
  },
  {
    line: '5316 #12',
    class: 'Class II',
    text:
      'Personnel record incomplete — CF-FSP 5337 mandated reporter acknowledgement missing (Handbook § 7.4).',
  },
  {
    line: '5316 #14',
    class: 'Class II',
    text:
      'Child immunization record (DH 680) not on file within 30 days of enrollment (Handbook § 7.1, § 21).',
  },
  {
    line: '5316 #15',
    class: 'Class II',
    text:
      'Student health examination (DH 3040) not on file or expired (Handbook § 7.2).',
  },
  {
    line: '5316 #07',
    class: 'Class II',
    text:
      '40-hour Introductory Child Care Training not completed within 12 months of hire (Handbook § 4.1).',
  },
  {
    line: '5316 #08',
    class: 'Class II',
    text:
      '10-hour annual in-service training (CF-FSP 5268) not documented for current fiscal year (Handbook § 4.2.7).',
  },
  {
    line: '5316 #09',
    class: 'Class I',
    text:
      'No staff member with current pediatric CPR + First Aid on premises during operating hours (Handbook § 4.2.4).',
  },
  {
    line: '5316 #05',
    class: 'Class III',
    text:
      'Current DCF license not posted in a conspicuous location near the main entrance (Handbook § 2.1).',
  },
  {
    line: '5316 #04',
    class: 'Class III',
    text:
      'Florida Child Care Facility Minimum Standards not posted where parents can read them (Handbook § 19).',
  },
  {
    line: '5316 #16',
    class: 'Class III',
    text:
      'Weekly meal & snack menu not posted for parents (Handbook § 3.9.3(H)).',
  },
  {
    line: '5316 #06',
    class: 'Class III',
    text:
      'Daily activity schedule not posted in every classroom (Handbook § 2.6).',
  },
  {
    line: '5316 #01',
    class: 'Class II',
    text:
      'Staff-to-child ratio or maximum group size exceeded at time of inspection (F.S. § 402.305(4); Handbook § 2.3).',
  },
];

function StatusChip({ status }: { status: StatusPill }): JSX.Element {
  const palette: Record<StatusPill, string> = {
    'On file': 'bg-brand-50 text-brand-800 border-brand-200',
    'Expiring soon': 'bg-amber-50 text-amber-900 border-amber-200',
    Expired: 'bg-critical-50 text-critical-700 border-critical-200',
    Missing: 'bg-stone-100 text-stone-700 border-stone-300',
  };
  return (
    <span
      className={`inline-flex items-center gap-1 px-2 py-0.5 rounded border text-xs font-semibold tracking-wide ${palette[status]}`}
    >
      {status === 'Expired' || status === 'Missing' ? (
        <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
      ) : null}
      {status}
    </span>
  );
}

function ItemRow({ item }: { item: PersonnelItem }): JSX.Element {
  return (
    <div className="flex items-start justify-between py-2 border-b border-inspector-rule last:border-b-0">
      <div className="min-w-0 pr-3">
        <div className="text-sm font-medium text-inspector-ink">
          {item.label}
        </div>
        {item.form_name ? (
          <div className="text-xs text-inspector-muted mt-0.5">
            {item.form_name}
          </div>
        ) : null}
        {item.expires_at ? (
          <div className="text-xs text-inspector-muted mt-0.5">
            Expires {item.expires_at}
          </div>
        ) : null}
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <StatusChip status={item.status} />
        {item.document_id ? (
          <a
            href={`/documents/${item.document_id}`}
            className="text-xs underline text-inspector-ink-soft hover:text-inspector-ink"
            target="_blank"
            rel="noreferrer"
          >
            Source PDF
          </a>
        ) : null}
      </div>
    </div>
  );
}

export default function InspectorView(): JSX.Element {
  const [snapshot, setSnapshot] = useState<InspectorSnapshot | null>(null);
  const [tab, setTab] = useState<TabKey>('snapshot');
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sessionID, setSessionID] = useState<string | null>(null);
  const [findings, setFindings] = useState<Finding[]>([]);
  const navigate = useNavigate();

  const refresh = async () => {
    try {
      const next = await getSnapshot();
      setSnapshot(next);
      if (next.open_session_id) setSessionID(next.open_session_id);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  // Auto-open a session on first entry so the operator doesn't have to
  // think about lifecycle. The inspector hands back the iPad; we close
  // the session when "Export as Plan of Correction draft" is tapped.
  useEffect(() => {
    if (sessionID || !snapshot) return;
    let cancelled = false;
    void (async () => {
      try {
        const sess = await startSession({ type: 'routine' });
        if (!cancelled) setSessionID(sess.id);
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [snapshot, sessionID]);

  const closeAndReturn = () => {
    navigate('/dashboard');
  };

  if (error) {
    return (
      <div className="min-h-screen bg-inspector-paper text-inspector-ink p-8">
        <h1 className="text-2xl font-semibold mb-4">
          Inspector View — couldn't load
        </h1>
        <p className="text-inspector-muted mb-4">{error}</p>
        <button
          onClick={closeAndReturn}
          className="px-4 py-2 rounded bg-inspector-ink text-white"
        >
          Close Inspector View
        </button>
      </div>
    );
  }

  if (!snapshot) {
    return (
      <div className="min-h-screen bg-inspector-paper text-inspector-ink grid place-items-center">
        <p className="text-inspector-muted">Loading inspector view…</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-inspector-paper text-inspector-ink font-sans">
      <Header
        snapshot={snapshot}
        onClose={closeAndReturn}
        onMenu={() => setDrawerOpen((v) => !v)}
      />

      <div className="flex max-w-[1400px] mx-auto px-4 lg:px-8 py-6 gap-6">
        {/* Tab list — sticky left column on iPad / desktop. */}
        <nav
          className={`${
            drawerOpen ? 'block' : 'hidden'
          } lg:block fixed lg:sticky lg:top-6 inset-0 lg:inset-auto z-20 lg:z-auto bg-inspector-paper lg:bg-transparent overflow-y-auto lg:overflow-visible w-full lg:w-72 shrink-0 p-4 lg:p-0`}
          aria-label="Inspector tabs"
        >
          <ul className="space-y-1">
            {TABS.map((t) => (
              <li key={t.key}>
                <button
                  onClick={() => {
                    setTab(t.key);
                    setDrawerOpen(false);
                  }}
                  className={`w-full text-left px-4 py-3 rounded-md border transition-colors ${
                    tab === t.key
                      ? 'bg-inspector-ink text-white border-inspector-ink'
                      : 'bg-inspector-card border-inspector-rule hover:border-inspector-ink-soft text-inspector-ink'
                  }`}
                >
                  <div className="text-sm font-semibold">{t.label}</div>
                  <div
                    className={`text-xs ${
                      tab === t.key ? 'text-stone-200' : 'text-inspector-muted'
                    }`}
                  >
                    {t.subtitle}
                  </div>
                </button>
              </li>
            ))}
          </ul>
        </nav>

        {/* Right content column. */}
        <main className="min-w-0 flex-1">
          {tab === 'snapshot' && <SnapshotTab snapshot={snapshot} />}
          {tab === 'personnel' && (
            <PersonnelTab
              snapshot={snapshot}
              sessionID={sessionID}
              onSampled={refresh}
            />
          )}
          {tab === 'children' && (
            <ChildrenTab
              snapshot={snapshot}
              sessionID={sessionID}
              onSampled={refresh}
            />
          )}
          {tab === 'records' && <RecordsTab snapshot={snapshot} />}
          {tab === 'postings' && <PostingsTab snapshot={snapshot} />}
          {tab === 'drills' && <DrillsTab snapshot={snapshot} />}
          {tab === 'exit' && (
            <ExitTab
              snapshot={snapshot}
              sessionID={sessionID}
              findings={findings}
              setFindings={setFindings}
            />
          )}
        </main>
      </div>

      <footer className="border-t border-inspector-rule mt-12 py-6 text-center text-xs text-inspector-muted">
        Powered by Levelbrook · Records source: Levelbrook (digital)
      </footer>
    </div>
  );
}

function Header({
  snapshot,
  onClose,
  onMenu,
}: {
  snapshot: InspectorSnapshot;
  onClose: () => void;
  onMenu: () => void;
}): JSX.Element {
  const today = new Date().toLocaleDateString(undefined, {
    weekday: 'long',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
  return (
    <header className="bg-inspector-ink text-white border-b border-inspector-ink-soft">
      <div className="max-w-[1400px] mx-auto px-4 lg:px-8 py-4 flex items-center justify-between gap-4">
        <button
          aria-label="Toggle inspector tabs"
          onClick={onMenu}
          className="lg:hidden p-2 -ml-2 rounded hover:bg-inspector-ink-soft"
        >
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>
        <div className="min-w-0 flex-1">
          <div className="text-[11px] uppercase tracking-[0.2em] text-stone-300">
            Inspector View
          </div>
          <h1 className="text-lg lg:text-2xl font-semibold truncate">
            {snapshot.facility.name}
          </h1>
          <div className="text-xs lg:text-sm text-stone-300">
            License #{snapshot.facility.license_number || '—'} · {today}
          </div>
        </div>
        <button
          onClick={onClose}
          className="px-3 lg:px-4 py-2 rounded bg-white/10 hover:bg-white/20 text-sm font-semibold border border-white/20"
        >
          Close Inspector View
        </button>
      </div>
    </header>
  );
}

function SnapshotTab({ snapshot }: { snapshot: InspectorSnapshot }): JSX.Element {
  const fc = snapshot.facility;
  const li = snapshot.last_inspection;
  return (
    <div className="space-y-6">
      <Section title="Facility identity" subtitle="CF-FSP 5316 §A — Administration">
        <Grid>
          <Field label="Legal name" value={fc.name} />
          <Field label="License #" value={fc.license_number || '—'} />
          <Field label="License expiry" value={fc.license_expiry || '—'} />
          <Field label="Address" value={fc.address || '—'} />
          <Field label="Capacity" value={String(fc.capacity || '—')} />
          <Field label="Director" value={fc.director_name || '—'} />
          <Field label="State" value={fc.state_code} />
          <Field
            label="Records source"
            value="Levelbrook (digital)"
            emphasize
          />
        </Grid>
      </Section>

      <Section
        title="Last inspection"
        subtitle="Most recent CF-FSP 5316 visit on record"
      >
        {li ? (
          <Grid>
            <Field label="Started" value={li.started_at} />
            <Field label="Ended" value={li.ended_at || '— in progress'} />
            <Field
              label="Counselor"
              value={li.counselor_name || '(not provided)'}
            />
            <Field label="Type" value={li.type || 'routine'} />
            <Field
              label="Findings"
              value={String(li.findings_count)}
            />
          </Grid>
        ) : (
          <EmptyState text="No prior inspection events recorded in Levelbrook for this facility." />
        )}
      </Section>
    </div>
  );
}

function PersonnelTab({
  snapshot,
  sessionID,
  onSampled,
}: {
  snapshot: InspectorSnapshot;
  sessionID: string | null;
  onSampled: () => void | Promise<void>;
}): JSX.Element {
  const [count, setCount] = useState(3);
  const [busy, setBusy] = useState(false);

  const pull = async () => {
    if (!sessionID) return;
    setBusy(true);
    try {
      await pullSample(sessionID, { kind: 'staff', count });
      await onSampled();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      <Section
        title="Personnel sample"
        subtitle="CF-FSP 5316 §B · Handbook § 7.4 (49-item personnel file)"
      >
        <div className="flex flex-wrap items-center gap-3 mb-4">
          <div className="flex items-center gap-2">
            <label className="text-sm text-inspector-muted">Sample size</label>
            <select
              value={count}
              onChange={(e) => setCount(Number(e.target.value))}
              className="border border-inspector-rule rounded px-3 py-2 bg-white text-inspector-ink"
            >
              {[1, 2, 3, 4, 5].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </div>
          <button
            onClick={pull}
            disabled={busy || !sessionID}
            className="px-4 py-2 rounded bg-inspector-ink text-white text-sm font-semibold disabled:opacity-60 min-h-[48px]"
          >
            {busy ? 'Pulling sample…' : 'Pull random staff sample'}
          </button>
          <span className="text-xs text-inspector-muted">
            Active staff only — terminated and archived are excluded.
          </span>
        </div>

        {snapshot.staff_sample.length === 0 ? (
          <EmptyState text="No staff sample drawn yet — tap 'Pull random staff sample' above." />
        ) : (
          <div className="space-y-4">
            {snapshot.staff_sample.map((s) => (
              <StaffCardView key={s.id} card={s} />
            ))}
          </div>
        )}
      </Section>
    </div>
  );
}

function StaffCardView({ card }: { card: StaffCard }): JSX.Element {
  return (
    <article className="bg-inspector-card border border-inspector-rule rounded-md p-4">
      <header className="flex items-start justify-between mb-3 pb-3 border-b border-inspector-rule">
        <div>
          <h3 className="text-base font-semibold text-inspector-ink">
            {card.name}
          </h3>
          <p className="text-xs text-inspector-muted">
            {card.role || '—'} · Hired {card.hire_date || '—'}
          </p>
        </div>
        <p className="text-xs text-inspector-muted">
          Handbook § 7.4 ({card.items.length} items)
        </p>
      </header>
      <div>
        {card.items.map((it, idx) => (
          <ItemRow key={`${card.id}-${idx}`} item={it} />
        ))}
      </div>
    </article>
  );
}

function ChildrenTab({
  snapshot,
  sessionID,
  onSampled,
}: {
  snapshot: InspectorSnapshot;
  sessionID: string | null;
  onSampled: () => void | Promise<void>;
}): JSX.Element {
  const [count, setCount] = useState(3);
  const [busy, setBusy] = useState(false);

  const pull = async () => {
    if (!sessionID) return;
    setBusy(true);
    try {
      await pullSample(sessionID, { kind: 'child', count });
      await onSampled();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      <Section
        title="Children sample"
        subtitle="CF-FSP 5316 §C · Handbook § 7.3 enrollment file"
      >
        <div className="flex flex-wrap items-center gap-3 mb-4">
          <div className="flex items-center gap-2">
            <label className="text-sm text-inspector-muted">Sample size</label>
            <select
              value={count}
              onChange={(e) => setCount(Number(e.target.value))}
              className="border border-inspector-rule rounded px-3 py-2 bg-white text-inspector-ink"
            >
              {[1, 2, 3, 4, 5].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </div>
          <button
            onClick={pull}
            disabled={busy || !sessionID}
            className="px-4 py-2 rounded bg-inspector-ink text-white text-sm font-semibold disabled:opacity-60 min-h-[48px]"
          >
            {busy ? 'Pulling sample…' : 'Pull random child sample'}
          </button>
          <span className="text-xs text-inspector-muted">
            Active enrolment only — withdrawn / archived excluded.
          </span>
        </div>

        {snapshot.child_sample.length === 0 ? (
          <EmptyState text="No children sample drawn yet — tap 'Pull random child sample' above." />
        ) : (
          <div className="space-y-4">
            {snapshot.child_sample.map((c) => (
              <ChildCardView key={c.id} card={c} />
            ))}
          </div>
        )}
      </Section>
    </div>
  );
}

function ChildCardView({ card }: { card: ChildCard }): JSX.Element {
  const allergyStrip = (card.allergies || '').trim();
  const medicalStrip = (card.medical_notes || '').trim();
  return (
    <article className="bg-inspector-card border border-inspector-rule rounded-md p-4">
      <header className="flex items-start justify-between mb-3 pb-3 border-b border-inspector-rule">
        <div>
          <h3 className="text-base font-semibold text-inspector-ink">
            {card.name}
          </h3>
          <p className="text-xs text-inspector-muted">
            DOB {card.date_of_birth || '—'} · Enrolled{' '}
            {card.enrollment_date || '—'}
          </p>
        </div>
        <p className="text-xs text-inspector-muted">
          Handbook § 7.3 ({card.items.length} items)
        </p>
      </header>
      {(allergyStrip || medicalStrip) && (
        <div className="mb-3 px-3 py-2 rounded border border-amber-300 bg-amber-50 text-amber-900 text-xs">
          {allergyStrip && (
            <div>
              <span className="font-semibold">Allergies:</span> {allergyStrip}
            </div>
          )}
          {medicalStrip && (
            <div className="mt-1">
              <span className="font-semibold">Medical notes:</span>{' '}
              {medicalStrip}
            </div>
          )}
        </div>
      )}
      <div>
        {card.items.map((it, idx) => (
          <ItemRow key={`${card.id}-${idx}`} item={it} />
        ))}
      </div>
    </article>
  );
}

function RecordsTab({ snapshot }: { snapshot: InspectorSnapshot }): JSX.Element {
  return (
    <Section
      title="Facility records"
      subtitle="CF-FSP 5316 §D · 49-item rollup at the facility level"
    >
      <div className="bg-inspector-card border border-inspector-rule rounded-md">
        {snapshot.facility_records.map((row, idx) => (
          <div
            key={`${row.label}-${idx}`}
            className="flex items-start justify-between py-3 px-4 border-b border-inspector-rule last:border-b-0"
          >
            <div className="min-w-0 pr-3">
              <div className="text-sm font-medium text-inspector-ink">
                {row.label}
              </div>
              <div className="text-xs text-inspector-muted">
                {row.form_name || '—'}
              </div>
              {row.expires_at && (
                <div className="text-xs text-inspector-muted mt-0.5">
                  Expires {row.expires_at}
                </div>
              )}
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <StatusChip status={row.status} />
              {row.document_id ? (
                <a
                  href={`/documents/${row.document_id}`}
                  target="_blank"
                  rel="noreferrer"
                  className="text-xs underline text-inspector-ink-soft hover:text-inspector-ink"
                >
                  Source PDF
                </a>
              ) : null}
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}

function PostingsTab({ snapshot }: { snapshot: InspectorSnapshot }): JSX.Element {
  return (
    <Section
      title="Postings"
      subtitle="CF-FSP 5316 §E · Handbook § 19 — required wall postings"
    >
      <div className="grid sm:grid-cols-2 gap-3">
        {snapshot.postings.map((p) => (
          <div
            key={`${p.kind}-${p.room_id || 'all'}`}
            className="bg-inspector-card border border-inspector-rule rounded-md p-3 flex items-start gap-3"
          >
            {p.photo_path ? (
              <a
                href={p.photo_path}
                target="_blank"
                rel="noreferrer"
                className="block w-20 h-20 shrink-0 bg-stone-200 rounded border border-inspector-rule overflow-hidden"
              >
                <img
                  src={p.photo_path}
                  alt={p.label}
                  className="w-full h-full object-cover"
                />
              </a>
            ) : (
              <div className="w-20 h-20 shrink-0 bg-stone-100 rounded border border-dashed border-inspector-rule grid place-items-center text-[10px] text-inspector-muted text-center px-1">
                No photo on file
              </div>
            )}
            <div className="min-w-0">
              <div className="text-sm font-semibold text-inspector-ink">
                {p.label}
              </div>
              <div className="text-xs text-inspector-muted">
                {p.required ? 'Required' : 'Optional'}
                {p.room_id ? ` · Room ${p.room_id}` : ''}
              </div>
              <div className="text-xs text-inspector-muted mt-1">
                {p.last_verified_at
                  ? `Last verified ${p.last_verified_at}`
                  : 'No verification photo on file — please review.'}
              </div>
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}

function DrillsTab({ snapshot }: { snapshot: InspectorSnapshot }): JSX.Element {
  const counts = snapshot.drills.counts_by_kind || {};
  const entries = snapshot.drills.logged || [];

  // Build a 12-month calendar grid (months × kinds).
  const months = useMemo(() => {
    const today = new Date();
    const out: { key: string; label: string }[] = [];
    for (let i = 11; i >= 0; i--) {
      const d = new Date(today.getFullYear(), today.getMonth() - i, 1);
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
      out.push({
        key,
        label: d.toLocaleDateString(undefined, {
          month: 'short',
          year: '2-digit',
        }),
      });
    }
    return out;
  }, []);

  const heat = useMemo(() => {
    const map: Record<string, Record<string, number>> = {};
    for (const e of entries) {
      const d = new Date(e.drill_date);
      if (Number.isNaN(d.getTime())) continue;
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
      if (!map[e.drill_kind]) map[e.drill_kind] = {};
      map[e.drill_kind][key] = (map[e.drill_kind][key] || 0) + 1;
    }
    return map;
  }, [entries]);

  const kinds = Object.keys(heat).length
    ? Object.keys(heat)
    : ['fire', 'lockdown', 'tornado'];

  return (
    <div className="space-y-6">
      <Section
        title="Drills + walkthroughs"
        subtitle="Handbook § 3.8 — last 12 months"
      >
        {entries.length === 0 ? (
          <EmptyState text="No drills logged in the last 12 months — please review." />
        ) : (
          <>
            <div className="mb-4 flex flex-wrap gap-2">
              {Object.entries(counts).map(([k, n]) => (
                <span
                  key={k}
                  className="px-2 py-1 rounded border border-inspector-rule bg-inspector-card text-xs"
                >
                  <span className="font-semibold">{k}:</span> {n}
                </span>
              ))}
            </div>

            <div className="overflow-x-auto">
              <table className="text-xs w-full">
                <thead>
                  <tr>
                    <th className="text-left p-1 text-inspector-muted">Kind</th>
                    {months.map((m) => (
                      <th key={m.key} className="p-1 text-inspector-muted">
                        {m.label}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {kinds.map((kind) => (
                    <tr key={kind}>
                      <td className="p-1 font-medium text-inspector-ink">
                        {kind}
                      </td>
                      {months.map((m) => {
                        const n = heat[kind]?.[m.key] || 0;
                        return (
                          <td
                            key={m.key}
                            className={`p-1 text-center rounded ${
                              n > 0
                                ? 'bg-inspector-ink text-white font-semibold'
                                : 'bg-stone-100 text-inspector-muted'
                            }`}
                          >
                            {n || '·'}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="mt-6">
              <h4 className="text-sm font-semibold text-inspector-ink mb-2">
                Detail log
              </h4>
              <ul className="divide-y divide-inspector-rule bg-inspector-card border border-inspector-rule rounded-md">
                {entries.map((e) => (
                  <li key={e.id} className="px-3 py-2 text-sm">
                    <span className="font-medium text-inspector-ink">
                      {e.drill_date.slice(0, 10)}
                    </span>
                    <span className="mx-2 text-inspector-muted">·</span>
                    <span>{e.drill_kind}</span>
                    {e.notes ? (
                      <span className="block text-xs text-inspector-muted mt-0.5">
                        {e.notes}
                      </span>
                    ) : null}
                  </li>
                ))}
              </ul>
            </div>
          </>
        )}
      </Section>
    </div>
  );
}

function ExitTab({
  snapshot: _snapshot,
  sessionID,
  findings,
  setFindings,
}: {
  snapshot: InspectorSnapshot;
  sessionID: string | null;
  findings: Finding[];
  setFindings: (f: Finding[]) => void;
}): JSX.Element {
  const [picked, setPicked] = useState<number>(0);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [planURL, setPlanURL] = useState<string | null>(null);

  const submit = async () => {
    if (!sessionID) return;
    const std = STANDARD_FINDINGS[picked];
    setBusy(true);
    try {
      const res = await addFinding(sessionID, {
        line_number: std.line,
        class: std.class,
        text: std.text,
        note,
      });
      setFindings(res.findings);
      setNote('');
    } finally {
      setBusy(false);
    }
  };

  const exportPlan = async () => {
    if (!sessionID) return;
    setBusy(true);
    try {
      await endSession(sessionID);
      setPlanURL(planOfCorrectionURL(sessionID));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      <Section
        title="Exit conference"
        subtitle="CF-FSP 5316 §F · Counselor's findings + Plan of Correction draft"
      >
        <div className="bg-inspector-card border border-inspector-rule rounded-md p-4 space-y-4">
          <div>
            <label className="block text-sm font-medium text-inspector-ink mb-1">
              Standard finding
            </label>
            <select
              value={picked}
              onChange={(e) => setPicked(Number(e.target.value))}
              className="w-full border border-inspector-rule rounded px-3 py-2 bg-white text-sm"
            >
              {STANDARD_FINDINGS.map((f, i) => (
                <option key={i} value={i}>
                  {f.line} ({f.class}) — {f.text.slice(0, 70)}
                  {f.text.length > 70 ? '…' : ''}
                </option>
              ))}
            </select>
            <p className="text-xs text-inspector-muted mt-1">
              {STANDARD_FINDINGS[picked].text}
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium text-inspector-ink mb-1">
              Note (optional)
            </label>
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={2}
              className="w-full border border-inspector-rule rounded px-3 py-2 bg-white text-sm"
              placeholder="e.g. specific staff member, room, or correction date."
            />
          </div>
          <button
            onClick={submit}
            disabled={busy || !sessionID}
            className="px-4 py-2 rounded bg-inspector-ink text-white text-sm font-semibold disabled:opacity-60 min-h-[48px]"
          >
            {busy ? 'Adding finding…' : 'Add finding'}
          </button>
        </div>

        <div className="mt-6">
          <h4 className="text-sm font-semibold text-inspector-ink mb-2">
            Findings on this visit ({findings.length})
          </h4>
          {findings.length === 0 ? (
            <EmptyState text="No findings entered for this visit yet." />
          ) : (
            <ol className="space-y-2 list-decimal list-inside">
              {findings.map((f, i) => (
                <li
                  key={`${f.line_number}-${i}`}
                  className="bg-inspector-card border border-inspector-rule rounded-md px-3 py-2 text-sm"
                >
                  <div className="font-semibold text-inspector-ink">
                    {f.line_number}{' '}
                    {f.class && (
                      <span className="text-xs text-inspector-muted">
                        ({f.class})
                      </span>
                    )}
                  </div>
                  <div>{f.text}</div>
                  {f.note && (
                    <div className="text-xs text-inspector-muted mt-1">
                      Note: {f.note}
                    </div>
                  )}
                </li>
              ))}
            </ol>
          )}
        </div>

        <div className="mt-6 pt-6 border-t border-inspector-rule">
          <button
            onClick={exportPlan}
            disabled={busy || !sessionID}
            className="px-4 py-3 rounded bg-inspector-accent text-inspector-ink text-sm font-bold disabled:opacity-60 min-h-[48px]"
          >
            Export as Plan of Correction draft
          </button>
          {planURL && (
            <a
              href={planURL}
              target="_blank"
              rel="noreferrer"
              className="ml-3 inline-flex items-center px-4 py-3 rounded border-2 border-inspector-ink text-inspector-ink text-sm font-bold hover:bg-inspector-ink hover:text-white"
            >
              Download PDF
            </a>
          )}
          <p className="text-xs text-inspector-muted mt-2">
            The draft pre-fills the director's name and signature line. The
            director countersigns elsewhere — DCF process governs final
            submission.
          </p>
        </div>
      </Section>
    </div>
  );
}

// ----------------------------------------------------------------------------
// Reusable bits
// ----------------------------------------------------------------------------

function Section({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}): JSX.Element {
  return (
    <section className="space-y-3">
      <header>
        <h2 className="text-xl lg:text-2xl font-semibold text-inspector-ink">
          {title}
        </h2>
        {subtitle && (
          <p className="text-sm text-inspector-muted mt-0.5">{subtitle}</p>
        )}
        <div className="h-px bg-inspector-rule mt-3" />
      </header>
      <div>{children}</div>
    </section>
  );
}

function Grid({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-3 bg-inspector-card border border-inspector-rule rounded-md p-4">
      {children}
    </div>
  );
}

function Field({
  label,
  value,
  emphasize,
}: {
  label: string;
  value: string;
  emphasize?: boolean;
}): JSX.Element {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-[0.15em] text-inspector-muted">
        {label}
      </div>
      <div
        className={`text-sm mt-0.5 ${
          emphasize
            ? 'text-inspector-ink font-bold'
            : 'text-inspector-ink font-medium'
        }`}
      >
        {value}
      </div>
    </div>
  );
}

function EmptyState({ text }: { text: string }): JSX.Element {
  return (
    <div className="px-4 py-6 text-sm text-inspector-muted text-center bg-inspector-card border border-dashed border-inspector-rule rounded-md">
      {text}
    </div>
  );
}
