import type { ReactNode } from 'react';

interface PageHeaderProps {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  eyebrow?: ReactNode;
}

export default function PageHeader({
  title,
  description,
  actions,
  eyebrow,
}: PageHeaderProps): JSX.Element {
  return (
    <div className="flex flex-col sm:flex-row sm:items-end sm:justify-between gap-4 pb-6 border-b border-gray-100 mb-6">
      <div>
        {eyebrow && (
          <div className="text-xs font-medium text-brand-700 uppercase tracking-wide mb-1">
            {eyebrow}
          </div>
        )}
        <h1 className="text-2xl font-semibold text-gray-900 tracking-tight">{title}</h1>
        {description && <p className="mt-1 text-sm text-gray-600 max-w-2xl">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}
