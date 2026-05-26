import { type ReactNode, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import './Popover.css';

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
      <span ref={anchorRef} className="fg-popover-anchor">
        {trigger({ open, toggle })}
      </span>
      {open && pos
        ? createPortal(
            <div ref={overlayRef} className="fg-popover-overlay" style={pos} role="menu">
              {overlay({ close })}
            </div>,
            document.body
          )
        : null}
    </>
  );
}

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
      className="fg-popover-item"
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
