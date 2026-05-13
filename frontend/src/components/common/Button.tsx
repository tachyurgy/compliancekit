import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { Loader2 } from 'lucide-react';

type Variant = 'primary' | 'secondary' | 'ghost' | 'warm' | 'danger';
type Size = 'sm' | 'md' | 'lg';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  leftIcon?: ReactNode;
  rightIcon?: ReactNode;
  fullWidth?: boolean;
}

const base =
  'inline-flex items-center justify-center gap-2 font-medium rounded-lb-sm transition-all whitespace-nowrap ' +
  'disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-offset-2 ' +
  'hover:-translate-y-px active:translate-y-0';

const variants: Record<Variant, string> = {
  // Sage primary — matches marketing .btn. Soft inset shadow gives the
  // confident "pressable" feel without looking heavy.
  primary:
    'bg-brand-600 text-white shadow-[0_1px_2px_rgba(13,111,94,.25),inset_0_-1px_0_rgba(0,0,0,.12)] ' +
    'hover:bg-brand-700 hover:shadow-[0_6px_18px_-4px_rgba(13,111,94,.4),inset_0_-1px_0_rgba(0,0,0,.12)] ' +
    'focus-visible:outline-brand-600',
  secondary:
    'bg-white text-ink-900 border border-line hover:bg-surface-sunken hover:border-line ' +
    'focus-visible:outline-ink-500 shadow-card',
  ghost:
    'text-ink-800 hover:bg-surface-sunken focus-visible:outline-ink-500',
  // Copper accent — for moments that should feel friendly and warm
  // (e.g. "Start trial", "Add your first child"). Never for alerts.
  warm:
    'bg-warm-500 text-white shadow-[0_1px_2px_rgba(168,93,53,.25),inset_0_-1px_0_rgba(0,0,0,.12)] ' +
    'hover:bg-warm-600 focus-visible:outline-warm-500',
  // Coral — used sparingly. "Delete", "Revoke", "End trial".
  danger:
    'bg-critical-600 text-white shadow-[0_1px_2px_rgba(161,66,30,.25),inset_0_-1px_0_rgba(0,0,0,.12)] ' +
    'hover:bg-critical-700 focus-visible:outline-critical-600',
};

const sizes: Record<Size, string> = {
  // ≥48px tap target on md so mobile passes a11y without re-thinking.
  sm: 'h-9 px-3 text-sm',
  md: 'h-12 px-5 text-sm',
  lg: 'h-14 px-7 text-base rounded-lb',
};

const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    variant = 'primary',
    size = 'md',
    loading = false,
    leftIcon,
    rightIcon,
    fullWidth,
    className = '',
    children,
    disabled,
    ...rest
  },
  ref,
) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={`${base} ${variants[variant]} ${sizes[size]} ${fullWidth ? 'w-full' : ''} ${className}`}
      {...rest}
    >
      {loading ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> : leftIcon}
      <span>{children}</span>
      {!loading && rightIcon}
    </button>
  );
});

export default Button;
