import { useQuery } from '@tanstack/react-query';
import { Phone } from 'lucide-react';
import { attendanceApi } from '@/api/attendance';

// REQ-016: dashboard warnings strip + 9:30 AM absence cards.
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
        <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-emerald-100 text-emerald-800 text-sm">
          <span className="w-2 h-2 rounded-full bg-emerald-500" />
          All clear · {data.children_present.length} children · {data.staff_present.length} staff · floor needs {data.required_floor}
        </div>
      ) : (
        data.warnings.map((w) => (
          <div key={w.code} className="rounded-xl border border-orange-200 bg-orange-50 p-4">
            <div className="text-sm font-semibold text-orange-900">{w.headline}</div>
            <div className="text-sm text-orange-800 mt-1">{w.action}</div>
          </div>
        ))
      )}

      {data.absences.length > 0 && (
        <div className="rounded-xl border border-amber-200 bg-amber-50 p-4">
          <div className="text-xs uppercase tracking-wider text-amber-800 font-semibold mb-2">
            On the schedule today — not signed in yet
          </div>
          <ul className="space-y-2">
            {data.absences.map((a) => (
              <li key={a.child_id} className="flex items-center justify-between gap-3 text-sm">
                <div>
                  <span className="font-medium text-gray-900">{a.first_name} {a.last_name}</span>
                  {a.parent_name && <span className="text-gray-500"> · {a.parent_name}</span>}
                </div>
                {a.parent_phone ? (
                  <a href={`tel:${a.parent_phone}`} className="inline-flex items-center gap-1.5 text-sm font-medium text-amber-900 hover:text-amber-700">
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
