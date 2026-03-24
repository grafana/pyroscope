import { useRef, useState } from 'react';
import { Button } from '@components/core/Button';
import { DropdownItem } from '@components/core/Dropdown';
import { Icon } from '@components/core/Icon';
import { useClickOutside } from '@hooks/useClickOutside';

export type SelectOption = {
  label: string;
  value: string;
  divider?: boolean;
};

const MENU_MIN_WIDTH = 160;

export function Select({
  value,
  options,
  onChange,
}: {
  value: string;
  options: SelectOption[];
  onChange: (v: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [menuAlign, setMenuAlign] = useState<'left' | 'right'>('left');
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => setOpen(false));
  const label = options.find((o) => o.value === value)?.label ?? value;

  const handleOpen = () => {
    if (!open && ref.current) {
      const rect = ref.current.getBoundingClientRect();
      setMenuAlign(rect.left + MENU_MIN_WIDTH > window.innerWidth ? 'right' : 'left');
    }
    setOpen((o) => !o);
  };

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <Button onClick={handleOpen} active={open}>
        {label}
        <Icon name="chevron-down" size={11} />
      </Button>

      {open && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            ...(menuAlign === 'left' ? { left: 0 } : { right: 0 }),
            zIndex: 1000,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-medium)',
            borderRadius: 'var(--radius-lg)',
            boxShadow: 'var(--shadow-md)',
            minWidth: 160,
            overflow: 'hidden',
            padding: 'var(--space-1) 0',
          }}
        >
          {options.map((opt) => (
            <div key={opt.value}>
              {opt.divider && (
                <div
                  style={{
                    height: 1,
                    background: 'var(--border-weak)',
                    margin: 'var(--space-1) 0',
                  }}
                />
              )}
              <DropdownItem
                selected={opt.value === value}
                onClick={() => {
                  onChange(opt.value);
                  setOpen(false);
                }}
              >
                <span>{opt.label}</span>
                {opt.value === value && <Icon name="check" size={12} />}
              </DropdownItem>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
