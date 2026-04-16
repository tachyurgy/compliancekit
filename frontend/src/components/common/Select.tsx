import { forwardRef, type SelectHTMLAttributes } from 'react';

interface Option {
  value: string;
  label: string;
}

interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  hint?: string;
  error?: string;
  options: Option[];
  placeholder?: string;
}

const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { label, hint, error, options, placeholder, className = '', id, ...rest },
  ref,
) {
  const selectId = id ?? rest.name;
  return (
    <div className="space-y-1">
      {label && (
        <label htmlFor={selectId} className="block text-sm font-medium text-gray-800">
          {label}
        </label>
      )}
      <select
        id={selectId}
        ref={ref}
        aria-invalid={!!error}
        className={`block w-full h-10 rounded-lg border bg-white text-sm px-3 ${
          error
            ? 'border-critical-500 focus:border-critical-600 focus:ring-critical-500'
            : 'border-gray-200 focus:border-brand-600 focus:ring-brand-600'
        } focus:outline-none focus:ring-2 focus:ring-opacity-20 ${className}`}
        {...rest}
      >
        {placeholder && (
          <option value="" disabled>
            {placeholder}
          </option>
        )}
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      {hint && !error && <p className="text-xs text-gray-500">{hint}</p>}
      {error && <p className="text-xs text-critical-600">{error}</p>}
    </div>
  );
});

export default Select;
