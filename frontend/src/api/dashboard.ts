import { z } from 'zod';
import { apiFetch } from './client';

export const AlertSeveritySchema = z.enum(['info', 'warning', 'critical']);

export const DashboardAlertSchema = z.object({
  id: z.string(),
  severity: AlertSeveritySchema,
  title: z.string(),
  description: z.string(),
  href: z.string().nullable(),
  dueDate: z.string().nullable(),
  category: z.enum([
    'document',
    'immunization',
    'certification',
    'drill',
    'ratio',
    'posting',
    'other',
  ]),
});
export type DashboardAlert = z.infer<typeof DashboardAlertSchema>;

export const DashboardTimelineItemSchema = z.object({
  id: z.string(),
  date: z.string(),
  label: z.string(),
  category: z.string(),
  severity: AlertSeveritySchema,
  href: z.string().nullable(),
});
export type DashboardTimelineItem = z.infer<typeof DashboardTimelineItemSchema>;

export const DashboardSchema = z.object({
  complianceScore: z.number().min(0).max(100),
  scoreDelta: z.number().int(),
  updatedAt: z.string(),
  counts: z.object({
    children: z.number().int(),
    staff: z.number().int(),
    documents: z.number().int(),
    criticalAlerts: z.number().int(),
    warningAlerts: z.number().int(),
  }),
  alerts: z.array(DashboardAlertSchema),
  timeline: z.array(DashboardTimelineItemSchema),
});
export type DashboardData = z.infer<typeof DashboardSchema>;

export const dashboardApi = {
  async get(): Promise<DashboardData> {
    const data = await apiFetch<unknown>('/api/dashboard');
    return DashboardSchema.parse(data);
  },
};
