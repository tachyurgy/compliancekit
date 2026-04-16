import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { childrenApi, type Child } from '@/api/children';

export function useChildren() {
  return useQuery({
    queryKey: ['children'],
    queryFn: () => childrenApi.list(),
  });
}

export function useChild(id: string | undefined) {
  return useQuery({
    queryKey: ['child', id],
    queryFn: () => childrenApi.get(id!),
    enabled: !!id,
  });
}

export function useCreateChild() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<Child>) => childrenApi.create(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['children'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}

export function useUpdateChild(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<Child>) => childrenApi.update(id, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['children'] });
      void qc.invalidateQueries({ queryKey: ['child', id] });
    },
  });
}

export function useDeleteChild() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => childrenApi.remove(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['children'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}
