import { create } from 'zustand';
import { createPortal } from 'react-dom';
import { CheckCircle2, AlertTriangle, XCircle, Info, X } from 'lucide-react';
import type { ReactNode } from 'react';

type ToastVariant = 'success' | 'error' | 'warning' | 'info';

interface ToastItem {
  id: string;
  variant: ToastVariant;
  message: string;
  duration: number;
}

interface ToastStore {
  toasts: ToastItem[];
  push: (t: Omit<ToastItem, 'id' | 'duration'> & { duration?: number }) => string;
  dismiss: (id: string) => void;
}

const useToastStore = create<ToastStore>((set, get) => ({
  toasts: [],
  push: ({ variant, message, duration = 4000 }) => {
    const id = Math.random().toString(36).slice(2);
    set((s) => ({ toasts: [...s.toasts, { id, variant, message, duration }] }));
    if (duration > 0) {
      window.setTimeout(() => get().dismiss(id), duration);
    }
    return id;
  },
  dismiss: (id) => {
    set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) }));
  },
}));

export const toast = {
  success: (message: string) => useToastStore.getState().push({ variant: 'success', message }),
  error: (message: string) => useToastStore.getState().push({ variant: 'error', message }),
  warning: (message: string) => useToastStore.getState().push({ variant: 'warning', message }),
  info: (message: string) => useToastStore.getState().push({ variant: 'info', message }),
};

const iconByVariant: Record<ToastVariant, ReactNode> = {
  success: <CheckCircle2 className="h-5 w-5 text-brand-600" />,
  error: <XCircle className="h-5 w-5 text-critical-600" />,
  warning: <AlertTriangle className="h-5 w-5 text-caution-500" />,
  info: <Info className="h-5 w-5 text-blue-500" />,
};

export default function ToastViewport(): JSX.Element | null {
  const toasts = useToastStore((s) => s.toasts);
  const dismiss = useToastStore((s) => s.dismiss);

  if (typeof document === 'undefined') return null;

  return createPortal(
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-[320px] max-w-[calc(100vw-2rem)]">
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          className="flex items-start gap-3 bg-white rounded-lg shadow-elevated border border-gray-100 px-4 py-3"
        >
          <div>{iconByVariant[t.variant]}</div>
          <p className="text-sm text-gray-800 flex-1">{t.message}</p>
          <button
            onClick={() => dismiss(t.id)}
            aria-label="Dismiss"
            className="text-gray-400 hover:text-gray-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      ))}
    </div>,
    document.body,
  );
}
