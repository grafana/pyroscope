import { useState } from 'react';
import { Icon } from './core/Icon';

export function QueryBar({
  query,
  onQueryChange,
  onRun,
}: {
  query: string;
  onQueryChange: (q: string) => void;
  onRun: () => void;
}) {
  const [lastRun, setLastRun] = useState<string | null>(null);
  const dirty = lastRun === null || lastRun !== query;

  const handleRun = () => {
    setLastRun(query);
    onRun();
  };

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        height: 44,
        padding: '0 var(--space-3)',
        background: 'var(--bg-primary)',
        borderBottom: '1px solid var(--border-weak)',
        gap: 'var(--space-2)',
        flexShrink: 0,
      }}
    >
      <input
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && handleRun()}
        onFocus={(e) => {
          e.currentTarget.style.borderColor = 'var(--color-primary-border)';
          e.currentTarget.style.boxShadow = '0 0 0 2px var(--action-focus)';
        }}
        onBlur={(e) => {
          e.currentTarget.style.borderColor = 'var(--border-medium)';
          e.currentTarget.style.boxShadow = 'none';
        }}
        style={{
          flex: 1,
          height: 28,
          background: 'var(--bg-secondary)',
          color: 'var(--text-primary)',
          border: '1px solid var(--border-medium)',
          borderRadius: 'var(--radius-sm)',
          padding: '0 var(--space-3)',
          fontSize: 'var(--text-sm)',
          fontFamily: 'var(--font-mono)',
          outline: 'none',
          minWidth: 0,
          transition:
            'border-color var(--duration-base) var(--ease-out), box-shadow var(--duration-base) var(--ease-out)',
        }}
      />

      <button
        onClick={handleRun}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 'var(--space-1-5)',
          height: 28,
          padding: '0 var(--space-3)',
          background: 'var(--color-primary)',
          color: 'var(--color-primary-foreground)',
          border: '1px solid transparent',
          borderRadius: 'var(--radius-sm)',
          fontSize: 'var(--text-sm)',
          fontWeight: 'var(--weight-medium)',
          cursor: 'pointer',
          flexShrink: 0,
        }}
      >
        <Icon name={dirty ? 'play' : 'refresh'} size={10} />
        Run
      </button>
    </div>
  );
}
