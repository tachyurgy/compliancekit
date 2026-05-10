import type { StatusPill } from '@/api/staff';
import { STATUS_PILL_LABEL } from '@/api/staff';

// REQ-004 status pill — verbatim copy from active/product/brand-voice.md.
// We map each pill to a Badge variant so the colour palette stays
// consistent with the rest of the app:
//   on_file        → compliant (calm green)
//   expiring_soon  → warning   (amber)
//   expired        → critical  (coral)
//   missing        → critical  (gray-with-red dot in spec; using critical)
//   awaiting_upload → info     (blue)

const variantFor: Record<StatusPill, string> = {
  on_file: 'bg-brand-50 text-brand-700 ring-brand-200',
  expiring_soon: 'bg-caution-50 text-caution-600 ring-caution-400/50',
  expired: 'bg-critical-50 text-critical-700 ring-critical-500/40',
  missing: 'bg-gray-100 text-gray-700 ring-gray-300',
  awaiting_upload: 'bg-blue-50 text-blue-700 ring-blue-200',
};

interface Props {
  status: StatusPill;
  count?: number; // when set, shows "Missing 3" form
  size?: 'sm' | 'md';
}

export default function StatusPillBadge({ status, count, size = 'md' }: Props): JSX.Element {
  const label = count !== undefined && count > 0 && status !== 'on_file'
    ? `${STATUS_PILL_LABEL[status]} ${count}`
    : STATUS_PILL_LABEL[status];
  const sizing = size === 'sm' ? 'text-xs px-2 py-0.5' : 'text-sm px-2.5 py-1';
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full font-medium ring-1 ring-inset ${variantFor[status]} ${sizing}`}
    >
      {status === 'missing' && (
        <span className="h-1.5 w-1.5 rounded-full bg-critical-500" aria-hidden />
      )}
      {label}
    </span>
  );
}
