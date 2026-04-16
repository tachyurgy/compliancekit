import { Loader2 } from 'lucide-react';

interface LoadingSpinnerProps {
  label?: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizes = {
  sm: 'h-4 w-4',
  md: 'h-6 w-6',
  lg: 'h-8 w-8',
};

export default function LoadingSpinner({
  label = 'Loading...',
  size = 'md',
  className = '',
}: LoadingSpinnerProps): JSX.Element {
  return (
    <div className={`inline-flex items-center gap-2 text-gray-500 ${className}`} role="status">
      <Loader2 className={`${sizes[size]} animate-spin text-brand-600`} aria-hidden />
      <span className="text-sm">{label}</span>
    </div>
  );
}
