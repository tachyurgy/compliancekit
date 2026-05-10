import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { staffApi, type CreateStaffInput } from '@/api/staff';

export function useStaffList(filter?: { status?: 'passing' | 'missing' | 'expired' }) {
  return useQuery({
    queryKey: ['staff', filter ?? null],
    queryFn: () => staffApi.list(filter),
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
    mutationFn: (input: CreateStaffInput) => staffApi.create(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}

export function useUpdateStaff(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<CreateStaffInput>) => staffApi.update(id, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff'] });
      void qc.invalidateQueries({ queryKey: ['staff', id] });
    },
  });
}

export function useArchiveStaff() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => staffApi.archive(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff'] });
      void qc.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}

export function useSendReminder(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { reason?: 'missing' | 'manual'; doc_kind?: string; note?: string }) =>
      staffApi.sendReminder(id, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['staff', id] });
    },
  });
}
