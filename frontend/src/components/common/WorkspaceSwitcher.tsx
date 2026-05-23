// REQ-038: top-left workspace switcher pill. Sits in the sidebar header
// above the nav so it's the first thing a signed-in operator sees on
// every page. Tone follows brand: calm + cheery, yellow accent only when
// the active workspace is the demo one.

import { useEffect, useRef, useState } from 'react';
import { ChevronDown, Check } from 'lucide-react';
import {
  useActiveWorkspace,
  useSwitchWorkspace,
  useWorkspaces,
} from '@/hooks/useWorkspaces';

export default function WorkspaceSwitcher(): JSX.Element | null {
  const q = useWorkspaces();
  const active = useActiveWorkspace();
  const switcher = useSwitchWorkspace();
  const [open, setOpen] = useState(false);
  const popRef = useRef<HTMLDivElement | null>(null);

  // Close on outside click. Hand-rolled because this is the only popover
  // in the layout right now — a portal/Headless-UI dep would be overkill.
  useEffect(() => {
    if (!open) return undefined;
    const onDown = (e: MouseEvent) => {
      if (!popRef.current) return;
      if (!popRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, [open]);

  if (q.isLoading || !q.data) return null;
  if (!active) return null;

  const workspaces = q.data.workspaces;
  // Single-workspace users get a plain non-interactive pill — there's no
  // second facility to switch to, so opening the menu would just be a
  // confusing no-op.
  if (workspaces.length <= 1) {
    return (
      <div className="px-3 py-2 rounded-md bg-surface-sunken text-ink-800 text-sm font-medium truncate">
        {active.name}
      </div>
    );
  }

  return (
    <div className="relative" ref={popRef}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={`w-full flex items-center justify-between gap-2 px-3 py-2 rounded-md border text-sm font-medium transition-colors ${
          active.isDemo
            ? 'border-warm-300 bg-warm-50 text-ink-900 hover:bg-warm-100'
            : 'border-line bg-white text-ink-800 hover:bg-surface-sunken'
        }`}
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <span className="flex items-center gap-2 min-w-0">
          <span className="truncate">{active.name}</span>
          {active.isDemo && (
            <span className="shrink-0 text-[10px] uppercase tracking-wide font-bold text-warm-800 bg-warm-200 px-1.5 py-0.5 rounded">
              Demo
            </span>
          )}
        </span>
        <ChevronDown className="h-4 w-4 text-ink-500 shrink-0" />
      </button>

      {open && (
        <div
          role="menu"
          className="absolute left-0 right-0 mt-1 z-30 rounded-md border border-line bg-white shadow-card overflow-hidden"
        >
          {workspaces.map((w) => (
            <button
              key={w.id}
              type="button"
              role="menuitem"
              disabled={switcher.isPending}
              onClick={async () => {
                if (w.isActive) {
                  setOpen(false);
                  return;
                }
                try {
                  await switcher.mutateAsync(w.id);
                } finally {
                  setOpen(false);
                }
              }}
              className="w-full flex items-center justify-between gap-2 px-3 py-2 text-sm hover:bg-surface-sunken disabled:opacity-60 text-left"
            >
              <span className="flex items-center gap-2 min-w-0">
                <span className="truncate text-ink-800">{w.name}</span>
                {w.isDemo && (
                  <span className="shrink-0 text-[10px] uppercase tracking-wide font-bold text-warm-800 bg-warm-100 px-1.5 py-0.5 rounded">
                    Demo
                  </span>
                )}
              </span>
              {w.isActive && <Check className="h-4 w-4 text-good-600 shrink-0" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
