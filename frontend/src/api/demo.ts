// REQ-024 (Shayna tour). Read demo-seed status + nuke the seed when the
// operator hits "Make this mine" at the end of the tour.
import { apiFetch } from './client';

export interface DemoStatus {
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
