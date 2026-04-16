import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export default function EmptyState({
  icon,
  title,
  description,
  action,
  className = '',
}: EmptyStateProps): JSX.Element {
  return (
    <div
      className={`text-center py-12 px-6 border border-dashed border-gray-200 rounded-lg bg-surface-muted ${className}`}
    >
      {icon && (
        <div className="mx-auto mb-4 h-12 w-12 rounded-full bg-white shadow-card grid place-items-center text-gray-500">
          {icon}
        </div>
      )}
      <h3 className="text-base font-semibold text-gray-900">{title}</h3>
      {description && (
        <div className="mt-2 text-sm text-gray-600 max-w-md mx-auto">{description}</div>
      )}
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}
