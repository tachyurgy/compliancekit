import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Mail, MessageSquare, Send, CheckCircle2, AlertCircle } from 'lucide-react';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import EmptyState from '@/components/common/EmptyState';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { formatDate } from '@/lib/format';
import { messagesApi, type MessageItem, type MessageRecipientKind } from '@/api/messages';

interface Props {
  recipientKind: MessageRecipientKind;
  recipientId: string;
  composerHref: string;
}

// REQ-044: reusable message thread for StaffDetail + ChildDetail.
// Calls GET /api/messages?recipientKind=&recipientId= and renders the
// outgoing thread newest-first. "Send a message" jumps to the composer
// pre-filled for this contact.
export default function MessageHistory({ recipientKind, recipientId, composerHref }: Props): JSX.Element {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['messages', recipientKind, recipientId],
    queryFn: () => messagesApi.history(recipientKind, recipientId),
    enabled: !!recipientId,
  });

  return (
    <Card
      title="Messages"
      description="Every email and text Levelbrook sent on your behalf."
      action={
        <Link to={composerHref}>
          <Button size="sm" variant="secondary" leftIcon={<Send className="h-4 w-4" />}>
            Send a message
          </Button>
        </Link>
      }
      padded={false}
    >
      {isLoading ? (
        <div className="p-6">
          <LoadingSpinner label="Loading messages" />
        </div>
      ) : isError ? (
        <div className="p-6 text-sm text-ink-700">
          We couldn't load the message history. Try again in a moment.
        </div>
      ) : !data || data.length === 0 ? (
        <div className="p-6">
          <EmptyState
            title="Nothing sent yet"
            description="Tap 'Send a message' above to get a note out. It'll show up here."
          />
        </div>
      ) : (
        <ul className="divide-y divide-line-soft">
          {data.map((item) => (
            <MessageRow key={item.id} item={item} />
          ))}
        </ul>
      )}
    </Card>
  );
}

function MessageRow({ item }: { item: MessageItem }): JSX.Element {
  const Icon = item.channel === 'sms' ? MessageSquare : Mail;
  const okay = item.status === 'sent';
  const preview = (item.body || '').replace(/\s+/g, ' ').slice(0, 160);
  return (
    <li className="px-5 py-3 flex items-start gap-3">
      <div className="mt-0.5 shrink-0">
        <Icon className={`h-4 w-4 ${okay ? 'text-brand-700' : 'text-critical-600'}`} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 text-sm font-medium text-ink-900">
          {item.channel === 'sms' ? 'Text message' : item.subject || 'Email (no subject)'}
          {okay ? (
            <CheckCircle2 className="h-3.5 w-3.5 text-brand-700" />
          ) : (
            <AlertCircle className="h-3.5 w-3.5 text-critical-600" />
          )}
        </div>
        <div className="text-xs text-ink-500 mt-0.5">
          {item.senderName ? `From ${item.senderName} · ` : ''}
          {formatDate(item.sentAt || item.createdAt)}
          {item.status !== 'sent' && (
            <span className="ml-2 text-critical-700">
              {item.status}
              {item.error ? ` · ${item.error}` : ''}
            </span>
          )}
        </div>
        {preview && <p className="text-sm text-ink-700 mt-1.5 line-clamp-3">{preview}</p>}
      </div>
    </li>
  );
}
