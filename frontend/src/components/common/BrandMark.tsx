import { Link } from 'react-router-dom';

interface Props {
  to?: string;
  size?: 'sm' | 'md' | 'lg';
  wordmark?: boolean;
  className?: string;
}

const tileSizes = {
  sm: 'h-7 w-7 text-[13px] rounded-[8px]',
  md: 'h-9 w-9 text-[15px] rounded-[10px]',
  lg: 'h-12 w-12 text-[20px] rounded-[12px]',
};

const wordSizes = {
  sm: 'text-base',
  md: 'text-lg',
  lg: 'text-2xl',
};

// Levelbrook brand mark — sage gradient tile with "L" + optional Inter wordmark.
// Matches marketing/_shared.css .brand / .brand-mark exactly so the app and
// marketing site share one identity. Use `to` for a Link, omit for inline.
export default function BrandMark({
  to,
  size = 'md',
  wordmark = true,
  className = '',
}: Props): JSX.Element {
  const body = (
    <span className={`inline-flex items-center gap-2.5 ${className}`}>
      <span
        className={`${tileSizes[size]} grid place-items-center font-bold text-white ` +
          `bg-gradient-to-br from-brand-600 to-brand-700 ` +
          `shadow-[inset_0_-2px_0_rgba(0,0,0,0.12),0_1px_2px_rgba(13,111,94,0.3)]`}
        aria-hidden
      >
        L
      </span>
      {wordmark && (
        <span className={`font-semibold tracking-tight text-ink-900 ${wordSizes[size]}`}>
          Levelbrook
        </span>
      )}
    </span>
  );
  if (to) {
    return (
      <Link to={to} className="inline-flex items-center hover:no-underline">
        {body}
      </Link>
    );
  }
  return body;
}
