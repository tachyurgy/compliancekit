import { useQuery } from '@tanstack/react-query';
import { scoreApi } from '@/api/score';

// REQ-013: live compliance score for the dashboard hero. Refetches on focus
// (default tanstack behaviour) so an operator who just uploaded a doc on
// another tab sees the score bump when they switch back. Backend caches the
// computation for 60s + busts on writes.
export function useScore() {
  return useQuery({
    queryKey: ['compliance-score'],
    queryFn: () => scoreApi.get(),
    staleTime: 30_000,
  });
}
