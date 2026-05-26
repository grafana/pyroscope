import { css } from '@emotion/css';
import { type ReactNode, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

/**
 * Click-to-open popover anchored to a trigger element. Closes on outside
 * click or Escape. Renders the overlay through a Portal so it's not clipped
 * by overflow:hidden parents. Position auto-flips if it would clip the
 * viewport bottom.
 */
export function Popover({
  trigger,
  overlay,
}: {
  trigger: (props: { open: boolean; toggle: () => void }) => ReactNode;
  overlay: (props: { close: () => void }) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const anchorRef = useRef<HTMLSpanElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => {
      const target = e.target as Node;
      if (overlayRef.current?.contains(target)) return;
      if (anchorRef.current?.contains(target)) return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  useEffect(() => {
    if (!open || !anchorRef.current) return;
    const rect = anchorRef.current.getBoundingClientRect();
    const wantsTop = rect.bottom + 240 > window.innerHeight && rect.top - 240 > 0;
    setPos({
      left: rect.left,
      top: wantsTop ? rect.top - 4 : rect.bottom + 4,
    });
  }, [open]);

  const toggle = () => setOpen((o) => !o);
  const close = () => setOpen(false);

  return (
    <>
      <span ref={anchorRef} className={styles.anchor}>
        {trigger({ open, toggle })}
      </span>
      {open && pos
        ? createPortal(
            <div ref={overlayRef} className={styles.overlay} style={pos} role="menu">
              {overlay({ close })}
            </div>,
            document.body
          )
        : null}
    </>
  );
}

const styles = {
  anchor: css({
    display: 'inline-block',
  }),
  overlay: css({
    position: 'fixed',
    zIndex: 1000,
    background: 'var(--bg-elevated)',
    border: '1px solid var(--border-medium)',
    borderRadius: 'var(--radius-md)',
    boxShadow: 'var(--shadow-md)',
    padding: '4px 0',
    minWidth: 160,
  }),
};

export function PopoverItem({
  label,
  onClick,
  active,
}: {
  label: string;
  onClick: () => void;
  active?: boolean;
}) {
  return (
    <div
      role="menuitem"
      tabIndex={0}
      className={popoverItemStyles.item}
      data-active={active ?? false}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick();
        }
      }}
    >
      {label}
    </div>
  );
}

const popoverItemStyles = {
  item: css({
    padding: '6px 12px',
    fontSize: 'var(--text-sm)',
    color: 'var(--text-primary)',
    cursor: 'pointer',
    whiteSpace: 'nowrap',
    '&:hover, &:focus': {
      background: 'var(--action-hover)',
      outline: 'none',
    },
    "&[data-active='true']": {
      background: 'var(--action-selected)',
      color: 'var(--color-primary-text)',
    },
  }),
};
