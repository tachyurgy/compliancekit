import type { HTMLAttributes, ReactNode } from 'react';

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
      className={`bg-white rounded-lb border border-line shadow-card transition-shadow ${className}`}
      {...rest}
    >
      {(title || action) && (
        <div className="px-6 py-5 border-b border-line-soft flex items-start justify-between gap-4">
          <div>
            {title && <h3 className="text-base font-semibold text-ink-900">{title}</h3>}
            {description && (
              <p className="mt-1 text-sm text-ink-500">{description}</p>
            )}
          </div>
          {action}
        </div>
      )}
      <div className={padded ? 'p-6' : ''}>{children}</div>
    </div>
  );
}
