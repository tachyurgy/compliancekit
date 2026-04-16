import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { staffApi, type Staff } from '@/api/staff';

export function useStaffList() {
  return useQuery({
    queryKey: ['staff'],
    queryFn: () => staffApi.list(),
  });
}

export function useStaffMember(id: string | undefined) {
  return useQuery({
    queryKey: ['staff', id],
    queryFn: () => staffApi.get(id!),
    enabled: !!id,
  });
}

export function useCreateStaff() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<Staff>) => staffApi.create(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}

export function useUpdateStaff(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<Staff>) => staffApi.update(id, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff'] });
      void qc.invalidateQueries({ queryKey: ['staff', id] });
    },
  });
}

export function useDeleteStaff() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => staffApi.remove(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}
