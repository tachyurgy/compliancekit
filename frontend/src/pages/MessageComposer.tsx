import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { Mail, MessageSquare, Send, X, ArrowLeft, CheckCircle2, AlertCircle } from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { toast } from '@/components/common/Toast';
import { messagesApi, type RecipientOption, type SendResult } from '@/api/messages';

// REQ-043: /messages/new composer. Recipients can be staff or parents,
// channel is email/sms/both, body is plaintext. Submit fires the
// fan-out and renders a per-recipient/per-channel results panel.

type Channel = 'email' | 'sms' | 'both';
type ChipKey = `${'staff' | 'parent'}:${string}`;

function chipKey(rec: { kind: 'staff' | 'parent'; id: string }): ChipKey {
  return `${rec.kind}:${rec.id}`;
}

export default function MessageComposer(): JSX.Element {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { data, isLoading, isError } = useQuery({
    queryKey: ['messages', 'recipients'],
    queryFn: () => messagesApi.recipients(),
  });

  // Pre-seed from query string. Supported:
  //   ?staffId=<id>  pre-selects that staff member
  //   ?parentId=<id> pre-selects that child's parent
  const initial = useMemo(() => {
    const next = new Set<ChipKey>();
    const s = params.get('staffId');
    const p = params.get('parentId');
    if (s) next.add(`staff:${s}`);
    if (p) next.add(`parent:${p}`);
    return next;
  }, [params]);

  const [selected, setSelected] = useState<Set<ChipKey>>(initial);
  const [channel, setChannel] = useState<Channel>('email');
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [sending, setSending] = useState(false);
  const [results, setResults] = useState<SendResult[] | null>(null);
  const [roleFilter, setRoleFilter] = useState<'all' | 'staff' | 'parents'>('all');

  const all: RecipientOption[] = useMemo(() => {
    if (!data) return [];
    return [...data.staff, ...data.parents];
  }, [data]);

  const lookup = useMemo(() => {
    const m = new Map<ChipKey, RecipientOption>();
    for (const r of all) m.set(`${r.kind}:${r.id}`, r);
    return m;
  }, [all]);

  const filtered = useMemo(() => {
    if (roleFilter === 'staff') return data?.staff ?? [];
    if (roleFilter === 'parents') return data?.parents ?? [];
    return all;
  }, [data, all, roleFilter]);

  function toggle(rec: RecipientOption) {
    const key = chipKey(rec);
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  function selectAll(kind: 'staff' | 'parents') {
    if (!data) return;
    const list = kind === 'staff' ? data.staff : data.parents;
    setSelected((prev) => {
      const next = new Set(prev);
      for (const r of list) next.add(chipKey(r));
      return next;
    });
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (selected.size === 0) {
      toast.error('Pick at least one recipient.');
      return;
    }
    if (body.trim() === '') {
      toast.error('Add a message before sending.');
      return;
    }
    setSending(true);
    try {
      const out = await messagesApi.send({
        recipients: Array.from(selected).map((k) => {
          const [kind, id] = k.split(':') as ['staff' | 'parent', string];
          return { kind, id };
        }),
        channel,
        subject: channel === 'sms' ? undefined : subject,
        body,
      });
      setResults(out);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "We couldn't send that. Try again.");
    } finally {
      setSending(false);
    }
  }

  if (isLoading) {
    return (
      <div className="py-10">
        <LoadingSpinner label="Loading recipients" />
      </div>
    );
  }
  if (isError || !data) {
    return (
      <div className="py-10 text-center text-sm text-ink-700">
        We couldn't load the recipients list. Try again in a moment.
      </div>
    );
  }

  const selectedList = Array.from(selected)
    .map((k) => lookup.get(k as ChipKey))
    .filter((x): x is RecipientOption => !!x);
  const count = selectedList.length;
  const channelLabel = channel === 'both' ? 'email + text' : channel === 'sms' ? 'text' : 'email';
  const submitLabel = count === 0 ? 'Send' : `Send to ${count} ${count === 1 ? 'person' : 'people'} via ${channelLabel}`;

  if (results) {
    return (
      <div>
        <button
          type="button"
          onClick={() => navigate(-1)}
          className="inline-flex items-center gap-1 text-sm font-medium text-ink-700 hover:text-ink-900 mb-5"
        >
          <ArrowLeft className="h-4 w-4" /> Back
        </button>
        <PageHeader
          title="Message sent"
          description={`We tried to deliver this to ${count} ${count === 1 ? 'person' : 'people'}. Here's how it went.`}
        />
        <Card padded={false}>
          <ul className="divide-y divide-line-soft">
            {results.map((r) => (
              <li key={`${r.recipientId}:${r.channel}`} className="px-5 py-3 flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-medium text-ink-900">
                    {r.recipientName || `${r.recipientKind} ${r.recipientId}`}
                  </div>
                  <div className="text-xs text-ink-500 capitalize">
                    {r.channel === 'sms' ? 'Text' : 'Email'} · {r.status}
                    {r.error && ` · ${r.error}`}
                  </div>
                </div>
                {r.status === 'sent' ? (
                  <CheckCircle2 className="h-4 w-4 text-brand-700" />
                ) : (
                  <AlertCircle className="h-4 w-4 text-critical-600" />
                )}
              </li>
            ))}
          </ul>
        </Card>
        <div className="mt-4 flex gap-2">
          <Button variant="secondary" onClick={() => setResults(null)}>
            Send another
          </Button>
          <Link to="/chase">
            <Button variant="ghost">Back to Chase Mode</Button>
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div>
      <button
        type="button"
        onClick={() => navigate(-1)}
        className="inline-flex items-center gap-1 text-sm font-medium text-ink-700 hover:text-ink-900 mb-5"
      >
        <ArrowLeft className="h-4 w-4" /> Back
      </button>
      <PageHeader
        title="Send a message"
        description="Pick the people, pick the channel, write the note. We'll take it from here."
      />
      <form onSubmit={submit} className="space-y-6">
        <Card title="Who's this for?" description="Search or filter by role. Selected chips show up below.">
          <div className="flex flex-wrap items-center gap-2 mb-3">
            <button
              type="button"
              onClick={() => setRoleFilter('all')}
              className={chipBtn(roleFilter === 'all')}
            >
              All
            </button>
            <button
              type="button"
              onClick={() => setRoleFilter('staff')}
              className={chipBtn(roleFilter === 'staff')}
            >
              Staff
            </button>
            <button
              type="button"
              onClick={() => setRoleFilter('parents')}
              className={chipBtn(roleFilter === 'parents')}
            >
              Parents
            </button>
            <div className="ml-auto flex gap-2">
              <button
                type="button"
                onClick={() => selectAll('staff')}
                className="text-xs font-medium text-brand-700 hover:underline"
              >
                Add all staff
              </button>
              <button
                type="button"
                onClick={() => selectAll('parents')}
                className="text-xs font-medium text-brand-700 hover:underline"
              >
                Add all parents
              </button>
            </div>
          </div>

          {/* Selected chips */}
          {selectedList.length > 0 && (
            <div className="flex flex-wrap gap-2 mb-3 pb-3 border-b border-line-soft">
              {selectedList.map((r) => (
                <button
                  type="button"
                  key={chipKey(r)}
                  onClick={() => toggle(r)}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-brand-50 text-brand-700 text-xs font-medium"
                >
                  {r.name}
                  {r.childName && <span className="text-ink-500">· {r.childName}</span>}
                  <X className="h-3 w-3" />
                </button>
              ))}
            </div>
          )}

          {/* Picker */}
          <div className="max-h-72 overflow-y-auto border border-line-soft rounded-lb">
            {filtered.length === 0 ? (
              <div className="p-4 text-sm text-ink-500">No contacts in this group.</div>
            ) : (
              <ul className="divide-y divide-line-soft">
                {filtered.map((r) => {
                  const isOn = selected.has(chipKey(r));
                  return (
                    <li key={chipKey(r)}>
                      <label className="flex items-start gap-3 px-3 py-2 hover:bg-surface-sunken cursor-pointer">
                        <input
                          type="checkbox"
                          checked={isOn}
                          onChange={() => toggle(r)}
                          className="mt-1 accent-brand-600"
                        />
                        <div className="min-w-0 flex-1">
                          <div className="text-sm font-medium text-ink-900">{r.name}</div>
                          <div className="text-xs text-ink-500">
                            {r.kind === 'staff' ? r.role || 'Staff' : `Parent · child ${r.childName}`}
                            {' · '}
                            {r.email || 'no email'} · {r.phone || 'no phone'}
                          </div>
                        </div>
                      </label>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </Card>

        <Card title="How should we send it?">
          <div role="radiogroup" className="flex flex-wrap gap-2">
            <ChannelOption
              label="Email"
              icon={<Mail className="h-4 w-4" />}
              on={channel === 'email'}
              onClick={() => setChannel('email')}
            />
            <ChannelOption
              label="Text"
              icon={<MessageSquare className="h-4 w-4" />}
              on={channel === 'sms'}
              onClick={() => setChannel('sms')}
            />
            <ChannelOption
              label="Both"
              icon={
                <span className="inline-flex items-center -space-x-1">
                  <Mail className="h-4 w-4" />
                  <MessageSquare className="h-4 w-4" />
                </span>
              }
              on={channel === 'both'}
              onClick={() => setChannel('both')}
            />
          </div>
        </Card>

        <Card title="Your message">
          {channel !== 'sms' && (
            <Input
              label="Subject"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="School closed Friday"
              className="mb-3"
            />
          )}
          <label className="block text-sm font-medium text-ink-800 mb-1.5">Message</label>
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={8}
            placeholder={
              channel === 'sms'
                ? 'Short and friendly. Texts cap around 1500 characters.'
                : 'Hi everyone, just a heads-up: we will close at noon on Friday for staff training. Thanks!'
            }
            className="block w-full rounded-lb-sm border border-line px-3.5 py-2.5 text-sm placeholder:text-ink-400 focus:border-brand-600 focus:ring-2 focus:ring-brand-600/20 focus:outline-none"
          />
          <p className="mt-2 text-xs text-ink-500">
            Keep it plain English. Links you type will work in email. Texts are short and unbranded.
          </p>
        </Card>

        <div className="flex justify-end gap-3 pt-2">
          <Button variant="ghost" type="button" onClick={() => navigate(-1)} disabled={sending}>
            Cancel
          </Button>
          <Button type="submit" loading={sending} leftIcon={<Send className="h-4 w-4" />}>
            {submitLabel}
          </Button>
        </div>
      </form>
    </div>
  );
}

function chipBtn(on: boolean): string {
  return `px-3 py-1 rounded-full text-xs font-medium border ${
    on ? 'bg-brand-50 text-brand-700 border-brand-200' : 'bg-white text-ink-700 border-line'
  }`;
}

function ChannelOption({
  label,
  icon,
  on,
  onClick,
}: {
  label: string;
  icon: React.ReactNode;
  on: boolean;
  onClick: () => void;
}): JSX.Element {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={on}
      onClick={onClick}
      className={`inline-flex items-center gap-2 px-4 py-2 rounded-lb border text-sm font-medium transition-colors ${
        on
          ? 'bg-brand-50 text-brand-700 border-brand-200'
          : 'bg-white text-ink-700 border-line hover:bg-surface-sunken'
      }`}
    >
      {icon}
      {label}
    </button>
  );
}
