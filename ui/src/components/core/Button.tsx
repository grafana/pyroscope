import { useState } from 'react';

export function Button({
  children,
  onClick,
  active = false,
  title,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  active?: boolean;
  title?: string;
}) {
  const [hov, setHov] = useState(false);
  return (
    <button
      title={title}
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 'var(--space-2)',
        padding: 'var(--space-1-5) var(--space-3)',
        background: active
          ? 'var(--action-selected)'
          : hov
            ? 'var(--action-hover)'
            : 'transparent',
        color: active ? 'var(--color-primary-text)' : 'var(--text-primary)',
        border: `1px solid ${active ? 'var(--color-primary-border)' : 'transparent'}`,
        borderRadius: 'var(--radius-md)',
        fontSize: 'var(--text-md)',
        fontWeight: 'var(--weight-medium)',
        cursor: 'pointer',
        whiteSpace: 'nowrap',
        transition: 'background var(--duration-fast) var(--ease-out)',
      }}
    >
      {children}
    </button>
  );
}
