import { useState } from 'react';

export function DropdownItem({
  children,
  onClick,
  selected,
  mono,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  selected?: boolean;
  mono?: boolean;
}) {
  const [hov, setHov] = useState(false);
  return (
    <div
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: 'var(--space-1-5) var(--space-3)',
        fontSize: 'var(--text-md)',
        fontFamily: mono ? 'var(--font-mono)' : undefined,
        color: selected ? 'var(--color-primary-text)' : 'var(--text-primary)',
        background: selected
          ? 'var(--action-selected)'
          : hov
            ? 'var(--action-hover)'
            : 'transparent',
        cursor: 'pointer',
      }}
    >
      {children}
    </div>
  );
}

export function DropdownSection({ label }: { label: string }) {
  return (
    <div
      style={{
        padding: 'var(--space-1-5) var(--space-3) var(--space-1)',
        fontSize: 'var(--text-xs)',
        color: 'var(--text-secondary)',
        letterSpacing: 'var(--tracking-caps)',
        textTransform: 'uppercase' as const,
        borderBottom: '1px solid var(--border-weak)',
      }}
    >
      {label}
    </div>
  );
}
