import { useRef, useState } from 'react';
import { Button } from '@components/core/Button';
import { DropdownItem } from '@components/core/Dropdown';
import { Icon, type IconType } from '@components/core/Icon';
import { useClickOutside } from '@hooks/useClickOutside';
import './Select.css';

export type SelectOption = {
  label: string;
  value: string;
  icon?: IconType;
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
      setMenuAlign(
        rect.left + MENU_MIN_WIDTH > window.innerWidth ? 'right' : 'left',
      );
    }
    setOpen((o) => !o);
  };

  return (
    <div ref={ref} className="select">
      <Button onClick={handleOpen} active={open}>
        {label}
        <Icon name="angle-down" size={11} />
      </Button>

      {open && (
        <div
          className="select-menu"
          style={menuAlign === 'left' ? { left: 0 } : { right: 0 }}
        >
          {options.map((opt) => (
            <div key={opt.value}>
              {opt.divider && <div className="select-divider" />}
              <DropdownItem
                selected={opt.value === value}
                onClick={() => {
                  onChange(opt.value);
                  setOpen(false);
                }}
              >
                <span className="select-option-label">
                  {opt.icon && <Icon name={opt.icon} size={13} />}
                  {opt.label}
                </span>
                {opt.value === value && <Icon name="check" size={12} />}
              </DropdownItem>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
