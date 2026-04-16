import type { HTMLAttributes, ReactNode } from 'react';

type Variant = 'compliant' | 'warning' | 'critical' | 'neutral' | 'info';

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: Variant;
  icon?: ReactNode;
}

const variants: Record<Variant, string> = {
  compliant: 'bg-brand-50 text-brand-700 ring-brand-200',
  warning: 'bg-caution-50 text-caution-600 ring-caution-400/50',
  critical: 'bg-critical-50 text-critical-700 ring-critical-500/40',
  neutral: 'bg-gray-50 text-gray-700 ring-gray-200',
  info: 'bg-blue-50 text-blue-700 ring-blue-200',
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
      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ring-1 ring-inset ${variants[variant]} ${className}`}
      {...rest}
    >
      {icon}
      {children}
    </span>
  );
}
