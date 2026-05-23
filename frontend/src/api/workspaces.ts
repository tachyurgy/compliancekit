// REQ-038: workspace switcher. A signed-in user can belong to more than one
// provider (their real facility + a seeded demo facility). These calls back
// the top-left switcher in Layout.tsx.

import { apiFetch } from './client';

export interface Workspace {
  id: string;
  name: string;
  isDemo: boolean;
  isActive: boolean;
}

export interface WorkspacesResponse {
  workspaces: Workspace[];
  active_provider_id: string;
}

export const workspacesApi = {
  async list(): Promise<WorkspacesResponse> {
    return apiFetch<WorkspacesResponse>('/api/me/workspaces');
  },
  async setActive(providerId: string): Promise<{ active_provider_id: string }> {
    return apiFetch('/api/me/active-workspace', {
      method: 'POST',
      json: { providerId },
    });
  },
};
