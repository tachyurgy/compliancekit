import { forwardRef, type InputHTMLAttributes, type ReactNode } from 'react';

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  hint?: string;
  error?: string;
  leftIcon?: ReactNode;
}

const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, hint, error, leftIcon, className = '', id, ...rest },
  ref,
) {
  const inputId = id ?? rest.name;
  const describedBy = [error ? `${inputId}-error` : null, hint ? `${inputId}-hint` : null]
    .filter(Boolean)
    .join(' ') || undefined;
  return (
    <div className="space-y-1">
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-gray-800">
          {label}
        </label>
      )}
      <div className="relative">
        {leftIcon && (
          <span className="absolute inset-y-0 left-0 pl-3 flex items-center text-gray-400">
            {leftIcon}
          </span>
        )}
        <input
          id={inputId}
          ref={ref}
          aria-invalid={!!error}
          aria-describedby={describedBy}
          className={`block w-full h-10 rounded-lg border bg-white text-sm px-3 ${
            leftIcon ? 'pl-10' : ''
          } ${
            error
              ? 'border-critical-500 focus:border-critical-600 focus:ring-critical-500'
              : 'border-gray-200 focus:border-brand-600 focus:ring-brand-600'
          } focus:outline-none focus:ring-2 focus:ring-opacity-20 placeholder:text-gray-400 ${className}`}
          {...rest}
        />
      </div>
      {hint && !error && (
        <p id={`${inputId}-hint`} className="text-xs text-gray-500">
          {hint}
        </p>
      )}
      {error && (
        <p id={`${inputId}-error`} className="text-xs text-critical-600">
          {error}
        </p>
      )}
    </div>
  );
});

export default Input;
