import { useMemo } from 'react';

// MarkdownBox renders a small, fixed-height scrollable view of a Markdown
// document. It's deliberately minimal — we only need headings, paragraphs,
// bold, and bullet lists for the ToS / DPA. This avoids a runtime markdown
// dependency for an otherwise tiny use case.

interface Props {
  source: string;
  ariaLabel?: string;
  maxHeightClass?: string;
}

interface Block {
  kind: 'h1' | 'h2' | 'h3' | 'p' | 'ul';
  text?: string;
  items?: string[];
}

function parse(md: string): Block[] {
  const lines = md.replace(/\r\n/g, '\n').split('\n');
  const blocks: Block[] = [];
  let para: string[] = [];
  let list: string[] = [];

  const flushPara = () => {
    if (para.length) {
      blocks.push({ kind: 'p', text: para.join(' ') });
      para = [];
    }
  };
  const flushList = () => {
    if (list.length) {
      blocks.push({ kind: 'ul', items: list });
      list = [];
    }
  };

  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (line === '') {
      flushPara();
      flushList();
      continue;
    }
    const h = /^(#{1,3})\s+(.+)$/.exec(line);
    if (h) {
      flushPara();
      flushList();
      const level = h[1].length;
      blocks.push({ kind: (`h${level}` as 'h1' | 'h2' | 'h3'), text: h[2] });
      continue;
    }
    const li = /^[-*]\s+(.+)$/.exec(line);
    if (li) {
      flushPara();
      list.push(li[1]);
      continue;
    }
    flushList();
    para.push(line);
  }
  flushPara();
  flushList();
  return blocks;
}

function renderInline(text: string): JSX.Element[] {
  // Handles **bold** only. Anything else is plain text.
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return parts.map((p, i) => {
    const m = /^\*\*(.+)\*\*$/.exec(p);
    if (m) return <strong key={i} className="font-semibold text-ink-900">{m[1]}</strong>;
    return <span key={i}>{p}</span>;
  });
}

export default function MarkdownBox({
  source,
  ariaLabel,
  maxHeightClass = 'max-h-72',
}: Props): JSX.Element {
  const blocks = useMemo(() => parse(source), [source]);
  return (
    <div
      role="region"
      aria-label={ariaLabel}
      tabIndex={0}
      className={`${maxHeightClass} overflow-y-auto rounded-md border border-line bg-surface-muted px-4 py-3 text-sm text-ink-800 leading-relaxed space-y-3`}
    >
      {blocks.map((b, i) => {
        if (b.kind === 'h1')
          return <h3 key={i} className="text-base font-semibold text-ink-900">{renderInline(b.text!)}</h3>;
        if (b.kind === 'h2')
          return <h4 key={i} className="text-sm font-semibold text-ink-900 mt-2">{renderInline(b.text!)}</h4>;
        if (b.kind === 'h3')
          return <h5 key={i} className="text-sm font-medium text-ink-800 mt-2">{renderInline(b.text!)}</h5>;
        if (b.kind === 'ul')
          return (
            <ul key={i} className="list-disc pl-5 space-y-1">
              {b.items!.map((it, j) => (
                <li key={j}>{renderInline(it)}</li>
              ))}
            </ul>
          );
        return <p key={i}>{renderInline(b.text!)}</p>;
      })}
    </div>
  );
}
