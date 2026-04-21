import type { HTMLAttributes, ReactNode } from 'react';

// Omit `title` from HTMLAttributes because the native string `title` tooltip
// conflicts with our ReactNode header title; ours takes precedence and we
// never need the native tooltip here.
interface CardProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  padded?: boolean;
}

export default function Card({
  title,
  description,
  action,
  padded = true,
  className = '',
  children,
  ...rest
}: CardProps): JSX.Element {
  return (
    <div
      className={`bg-white rounded-lg border border-gray-100 shadow-card ${className}`}
      {...rest}
    >
      {(title || action) && (
        <div className="px-5 py-4 border-b border-gray-100 flex items-start justify-between gap-4">
          <div>
            {title && <h3 className="text-base font-semibold text-gray-900">{title}</h3>}
            {description && (
              <p className="mt-1 text-sm text-gray-600">{description}</p>
            )}
          </div>
          {action}
        </div>
      )}
      <div className={padded ? 'p-5' : ''}>{children}</div>
    </div>
  );
}
