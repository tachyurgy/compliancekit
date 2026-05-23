import { z } from 'zod';
import { apiFetch } from './client';

// REQ-039: free-form message composer. Three endpoints:
//
//   POST /api/messages/send            — fan out across recipients + channels
//   GET  /api/messages                 — per-recipient thread
//   GET  /api/messages/recipients      — picker list (staff + parents)

export const MessageRecipientKindSchema = z.enum(['staff', 'parent']);
export type MessageRecipientKind = z.infer<typeof MessageRecipientKindSchema>;

export const RecipientOptionSchema = z.object({
  id: z.string(),
  kind: MessageRecipientKindSchema,
  name: z.string(),
  role: z.string().optional().default(''),
  childName: z.string().optional().default(''),
  email: z.string().optional().default(''),
  phone: z.string().optional().default(''),
});
export type RecipientOption = z.infer<typeof RecipientOptionSchema>;

export const RecipientsResponseSchema = z.object({
  staff: z.array(RecipientOptionSchema),
  parents: z.array(RecipientOptionSchema),
});
export type RecipientsResponse = z.infer<typeof RecipientsResponseSchema>;

export const SendResultSchema = z.object({
  recipientKind: MessageRecipientKindSchema,
  recipientId: z.string(),
  recipientName: z.string().optional().default(''),
  channel: z.enum(['email', 'sms']),
  status: z.enum(['sent', 'failed', 'skipped', 'queued']),
  error: z.string().optional().default(''),
});
export type SendResult = z.infer<typeof SendResultSchema>;

export const MessageItemSchema = z.object({
  id: z.string(),
  channel: z.enum(['email', 'sms']),
  subject: z.string().optional().default(''),
  body: z.string(),
  status: z.string(),
  error: z.string().optional().default(''),
  sentAt: z.string().optional().default(''),
  createdAt: z.string(),
  senderName: z.string().optional().default(''),
});
export type MessageItem = z.infer<typeof MessageItemSchema>;

export interface SendInput {
  recipients: { kind: MessageRecipientKind; id: string }[];
  channel: 'email' | 'sms' | 'both';
  subject?: string;
  body: string;
}

export const messagesApi = {
  async recipients(): Promise<RecipientsResponse> {
    const data = await apiFetch<unknown>('/api/messages/recipients');
    return RecipientsResponseSchema.parse(data);
  },
  async send(input: SendInput): Promise<SendResult[]> {
    const data = await apiFetch<unknown>('/api/messages/send', {
      method: 'POST',
      json: input,
    });
    const parsed = z.object({ results: z.array(SendResultSchema) }).parse(data);
    return parsed.results;
  },
  async history(
    recipientKind: MessageRecipientKind,
    recipientId: string,
    limit = 50,
  ): Promise<MessageItem[]> {
    const data = await apiFetch<unknown>('/api/messages', {
      query: { recipientKind, recipientId, limit },
    });
    const parsed = z.object({ items: z.array(MessageItemSchema) }).parse(data);
    return parsed.items;
  },
};
