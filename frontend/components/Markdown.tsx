'use client';

import React from 'react';
import ReactMarkdown from 'react-markdown';

const styles: Record<string, React.CSSProperties> = {
  h1: { fontSize: '1.5em', fontWeight: 700, color: 'var(--text-primary)', margin: '0.6em 0 0.3em' },
  h2: { fontSize: '1.25em', fontWeight: 600, color: 'var(--text-primary)', margin: '0.5em 0 0.25em' },
  h3: { fontSize: '1.1em', fontWeight: 600, color: 'var(--text-primary)', margin: '0.4em 0 0.2em' },
  p: { color: 'var(--text-primary)', margin: '0.3em 0', lineHeight: 1.6 },
  code: { background: 'rgba(255,255,255,0.08)', padding: '2px 6px', borderRadius: 4, fontSize: '0.85em', fontFamily: 'monospace', color: 'var(--text-primary)' },
  pre: { background: 'rgba(0,0,0,0.3)', padding: '10px 14px', borderRadius: 8, overflowX: 'auto', fontSize: '0.8em', fontFamily: 'monospace', color: 'var(--text-primary)', margin: '0.4em 0' },
  ul: { color: 'var(--text-primary)', paddingLeft: '1.5em', margin: '0.3em 0' },
  ol: { color: 'var(--text-primary)', paddingLeft: '1.5em', margin: '0.3em 0' },
  li: { margin: '0.15em 0' },
  table: { width: '100%', borderCollapse: 'collapse', margin: '0.4em 0', fontSize: '0.85em' },
  th: { border: '1px solid var(--border-glass)', padding: '6px 10px', textAlign: 'left', background: 'rgba(255,255,255,0.04)', color: 'var(--text-secondary)', fontWeight: 600 },
  td: { border: '1px solid var(--border-glass)', padding: '6px 10px', color: 'var(--text-primary)' },
  a: { color: 'var(--accent)', textDecoration: 'underline' },
  hr: { borderColor: 'var(--border-glass)', margin: '0.6em 0' },
  blockquote: { borderLeft: '3px solid var(--accent)', paddingLeft: 12, margin: '0.4em 0', color: 'var(--text-secondary)', fontSize: '0.9em' },
  em: { color: 'var(--text-primary)' },
  strong: { color: 'var(--text-primary)', fontWeight: 700 },
};

// Custom components for ReactMarkdown.
const components: any = {
  h1: (props: any) => <h1 style={styles.h1}>{props.children}</h1>,
  h2: (props: any) => <h2 style={styles.h2}>{props.children}</h2>,
  h3: (props: any) => <h3 style={styles.h3}>{props.children}</h3>,
  p: (props: any) => <p style={styles.p}>{props.children}</p>,
  code: (props: any) => {
    if (props.inline) return <code style={styles.code}>{props.children}</code>;
    return <pre style={styles.pre}><code>{props.children}</code></pre>;
  },
  pre: (props: any) => <pre style={styles.pre}>{props.children}</pre>,
  ul: (props: any) => <ul style={styles.ul}>{props.children}</ul>,
  ol: (props: any) => <ol style={styles.ol}>{props.children}</ol>,
  li: (props: any) => <li style={styles.li}>{props.children}</li>,
  table: (props: any) => <table style={styles.table}>{props.children}</table>,
  th: (props: any) => <th style={styles.th}>{props.children}</th>,
  td: (props: any) => <td style={styles.td}>{props.children}</td>,
  a: (props: any) => <a style={styles.a} href={props.href} target="_blank" rel="noopener noreferrer">{props.children}</a>,
  hr: (props: any) => <hr style={styles.hr} />,
  blockquote: (props: any) => <blockquote style={styles.blockquote}>{props.children}</blockquote>,
  em: (props: any) => <em style={styles.em}>{props.children}</em>,
  strong: (props: any) => <strong style={styles.strong}>{props.children}</strong>,
};

interface MarkdownProps {
  children: string;
  className?: string;
}

export default function Markdown({ children, className }: MarkdownProps) {
  if (!children) return null;
  return (
    <div className={className}>
      <ReactMarkdown components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}