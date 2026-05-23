// REQ-024 + REQ-038. /api/demo/status reports whether the current active
// workspace is the demo one and counts the per-row is_demo flags we still
// emit for back-compat with the pre-REQ-038 tour beats. /api/demo/purge
// only acts when the active workspace is demo — see backend handler doc.
import { apiFetch } from './client';

export interface DemoStatus {
  // REQ-038: the active workspace is providers.is_demo=1.
  is_demo_workspace: boolean;
  has_demo_data: boolean;
  demo_staff: number;
  demo_children: number;
  demo_rooms: number;
}

export const demoApi = {
  async status(): Promise<DemoStatus> {
    return apiFetch<DemoStatus>('/api/demo/status');
  },
  async purge(): Promise<{ status: string; has_demo_data: boolean }> {
    return apiFetch('/api/demo/purge', { method: 'POST' });
  },
};
