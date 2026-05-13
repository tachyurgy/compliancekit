import { useQuery } from '@tanstack/react-query';
import { Phone, CheckCircle2 } from 'lucide-react';
import { attendanceApi } from '@/api/attendance';

// REQ-016 / REQ-012c: dashboard warnings strip + 9:30 AM absence cards.
//
// Quiet "all clear" pill when nothing's wrong. Calm coral cards (never red)
// when one of the four warnings fires — each with a one-sentence
// prescriptive action. The absence list renders below as tap-to-call cards.
export default function AttendanceStrip(): JSX.Element | null {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['attendance-today'],
    queryFn: () => attendanceApi.today(),
    refetchInterval: 30_000,
  });

  if (isLoading || isError || !data) return null;

  const allClear = data.warnings.length === 0;

  return (
    <div className="space-y-3 mb-6">
      {allClear ? (
        <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-brand-50 text-brand-800 text-sm font-medium ring-1 ring-brand-100">
          <CheckCircle2 className="h-4 w-4 text-brand-600" aria-hidden />
          All clear · {data.children_present.length} children · {data.staff_present.length} staff · floor needs {data.required_floor}
        </div>
      ) : (
        data.warnings.map((w) => (
          <div
            key={w.code}
            className="rounded-lb border border-critical-200 bg-critical-50/80 p-4"
            role="alert"
          >
            <div className="text-sm font-semibold text-critical-700">{w.headline}</div>
            <div className="text-sm text-ink-800 mt-1 leading-relaxed">{w.action}</div>
          </div>
        ))
      )}

      {data.absences.length > 0 && (
        <div className="rounded-lb border border-caution-100 bg-caution-50/70 p-4">
          <div className="text-xs uppercase tracking-[0.12em] text-caution-700 font-semibold mb-2.5">
            On the schedule today — not signed in yet
          </div>
          <ul className="space-y-2">
            {data.absences.map((a) => (
              <li
                key={a.child_id}
                className="flex items-center justify-between gap-3 text-sm"
              >
                <div>
                  <span className="font-medium text-ink-900">
                    {a.first_name} {a.last_name}
                  </span>
                  {a.parent_name && <span className="text-ink-500"> · {a.parent_name}</span>}
                </div>
                {a.parent_phone ? (
                  <a
                    href={`tel:${a.parent_phone}`}
                    className="inline-flex items-center gap-1.5 text-sm font-medium text-caution-700 hover:text-caution-600"
                  >
                    <Phone className="h-4 w-4" />
                    {a.parent_phone}
                  </a>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
