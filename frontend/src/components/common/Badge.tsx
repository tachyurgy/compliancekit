import type { HTMLAttributes, ReactNode } from 'react';

type Variant = 'compliant' | 'warning' | 'critical' | 'neutral' | 'info';

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: Variant;
  icon?: ReactNode;
}

// Pill palette aligned to operator brand:
//   compliant  → sage tints (calm, not lime)
//   warning    → amber (expiring soon)
//   critical   → coral, not red (per brand-voice — "let's fix this")
//   neutral    → cream + ink (matches surface)
//   info       → sky tint (used for "awaiting upload")
const variants: Record<Variant, string> = {
  compliant: 'bg-brand-50 text-brand-700 ring-brand-200',
  warning: 'bg-caution-50 text-caution-700 ring-caution-400/50',
  critical: 'bg-critical-50 text-critical-700 ring-critical-200',
  neutral: 'bg-surface-sunken text-ink-700 ring-line',
  info: 'bg-[#eaf2f7] text-[#3a6b8a] ring-[#cdd9e2]',
};

export default function Badge({
  variant = 'neutral',
  icon,
  className = '',
  children,
  ...rest
}: BadgeProps): JSX.Element {
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium ring-1 ring-inset ${variants[variant]} ${className}`}
      {...rest}
    >
      {icon}
      {children}
    </span>
  );
}
