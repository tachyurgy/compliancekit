// REQ-038: yellow sticky banner that confirms the operator is inside the
// demo workspace. Copy is 7th-grade reading level: short sentences, no
// jargon, friendly tone. The "Switch to your real facility" link finds
// the user's non-demo workspace and flips into it.

import { useActiveWorkspace, useSwitchWorkspace, useWorkspaces } from '@/hooks/useWorkspaces';
import { ArrowRight, Sparkles } from 'lucide-react';

export default function DemoBanner(): JSX.Element | null {
  const q = useWorkspaces();
  const active = useActiveWorkspace();
  const switcher = useSwitchWorkspace();

  if (!active || !active.isDemo) return null;
  if (!q.data) return null;

  const realWorkspace = q.data.workspaces.find((w) => !w.isDemo);

  return (
    <div className="sticky top-0 z-40 w-full bg-warm-100 border-b border-warm-300 text-ink-900">
      <div className="page-container flex items-center justify-between gap-3 py-2 text-sm">
        <div className="flex items-center gap-2 min-w-0">
          <Sparkles className="h-4 w-4 text-warm-700 shrink-0" />
          <span className="truncate">
            <strong className="font-semibold">You're in Demo mode.</strong>{' '}
            This is a practice facility. Anything you do here won't touch
            your real files.
          </span>
        </div>
        {realWorkspace && (
          <button
            type="button"
            disabled={switcher.isPending}
            onClick={() => switcher.mutate(realWorkspace.id)}
            className="shrink-0 inline-flex items-center gap-1 px-2.5 py-1 rounded-md bg-white border border-warm-300 text-warm-800 font-semibold hover:bg-warm-50 disabled:opacity-60"
          >
            Switch to your real facility
            <ArrowRight className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
    </div>
  );
}
