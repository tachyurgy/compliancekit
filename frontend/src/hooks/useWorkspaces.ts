// REQ-038: workspace switcher. Backed by /api/me/workspaces +
// /api/me/active-workspace. We surface a single hook so the layout pill
// and the demo banner both read off the same Tanstack cache.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { workspacesApi, type Workspace } from '@/api/workspaces';

export const WORKSPACES_QUERY_KEY = ['me', 'workspaces'] as const;

export function useWorkspaces() {
  return useQuery({
    queryKey: WORKSPACES_QUERY_KEY,
    queryFn: () => workspacesApi.list(),
    staleTime: 30_000,
  });
}

export function useActiveWorkspace(): Workspace | null {
  const q = useWorkspaces();
  if (!q.data) return null;
  const active =
    q.data.workspaces.find((w) => w.isActive) ??
    q.data.workspaces.find((w) => w.id === q.data.active_provider_id) ??
    null;
  return active;
}

// useSwitchWorkspace returns a mutation that POSTs to the backend and then
// invalidates every cache that depends on the active provider. The list
// is opinionated: we wipe everything except the workspaces list itself
// (which we refetch) because every API in the app is provider-scoped.
export function useSwitchWorkspace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (providerId: string) => workspacesApi.setActive(providerId),
    onSuccess: async () => {
      // Refetch workspaces so the pill flips immediately.
      await qc.invalidateQueries({ queryKey: WORKSPACES_QUERY_KEY });
      // Nuke every other provider-scoped cache. Tanstack Query treats
      // an empty predicate match as "everything" — we exclude the
      // workspaces key explicitly so the pill animation doesn't tear.
      await qc.invalidateQueries({
        predicate: (q) => {
          const key = q.queryKey[0];
          return key !== WORKSPACES_QUERY_KEY[0] || q.queryKey[1] !== WORKSPACES_QUERY_KEY[1];
        },
      });
    },
  });
}
