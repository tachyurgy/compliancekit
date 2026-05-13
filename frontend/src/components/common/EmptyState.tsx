import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}

// Warm, encouraging empty state. Cream background, sage icon halo,
// generous whitespace — never the "nothing here yet" cold-page feeling.
// Title should be the encouraging line from brand-voice.md.
export default function EmptyState({
  icon,
  title,
  description,
  action,
  className = '',
}: EmptyStateProps): JSX.Element {
  return (
    <div
      className={`text-center py-16 px-6 rounded-lb border border-line bg-surface-sunken/60 ${className}`}
    >
      {icon && (
        <div
          className="mx-auto mb-5 h-14 w-14 rounded-full bg-white shadow-card grid place-items-center text-brand-600 ring-1 ring-brand-100"
          aria-hidden
        >
          {icon}
        </div>
      )}
      <h3 className="font-display text-xl text-ink-900">{title}</h3>
      {description && (
        <div className="mt-3 text-sm text-ink-700 leading-relaxed max-w-md mx-auto">
          {description}
        </div>
      )}
      {action && <div className="mt-6 flex justify-center">{action}</div>}
    </div>
  );
}
