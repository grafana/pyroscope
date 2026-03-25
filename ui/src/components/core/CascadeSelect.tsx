import { useRef, useState } from 'react';
import { Button } from '@components/core/Button';
import { DropdownItem, DropdownSection } from '@components/core/Dropdown';
import { Icon } from '@components/core/Icon';
import { useClickOutside } from '@hooks/useClickOutside';

type CascadeItem = { label: string; value: string };
type CascadeGroup = { label: string; value: string; items: CascadeItem[] };

export function CascadeSelect({
  groups,
  groupLabel,
  itemLabel,
  value,
  onChange,
  loading = false,
}: {
  groups: CascadeGroup[];
  groupLabel: string;
  itemLabel: string;
  value: { group: string; item: string };
  onChange: (group: string, item: string) => void;
  loading?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [hovGroup, setHovGroup] = useState(value.group);
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => setOpen(false));

  const noData = !loading && groups.length === 0;
  const disabled = loading || noData;

  const handleOpen = () => {
    if (disabled) return;
    setHovGroup(value.group);
    setOpen((o) => !o);
  };

  const selectedGroupLabel =
    groups.find((g) => g.value === value.group)?.label ?? value.group;
  const selectedItemLabel =
    groups
      .find((g) => g.value === value.group)
      ?.items.find((i) => i.value === value.item)?.label ?? value.item;

  const hovItems = groups.find((g) => g.value === hovGroup)?.items ?? [];

  const buttonContent = loading ? (
    <>
      <style>{`@keyframes cs-spin{to{transform:rotate(360deg)}}`}</style>
      <span style={{
        width: 12, height: 12, flexShrink: 0,
        border: '1.5px solid var(--border-medium)',
        borderTopColor: 'var(--text-secondary)',
        borderRadius: '50%',
        animation: 'cs-spin 0.7s linear infinite',
        display: 'inline-block',
      }} />
      Loading
    </>
  ) : noData ? (
    <span style={{ color: 'var(--text-disabled)' }}>No data</span>
  ) : (
    <>
      {selectedGroupLabel} · {selectedItemLabel}
      <Icon name="chevron-down" size={11} />
    </>
  );

  return (
    <div ref={ref} style={{ position: 'relative', opacity: disabled ? 0.6 : 1 }}>
      <Button onClick={handleOpen} active={open}>
        {buttonContent}
      </Button>

      {open && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            zIndex: 1000,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-medium)',
            borderRadius: 'var(--radius-lg)',
            boxShadow: 'var(--shadow-md)',
            display: 'flex',
            minWidth: 340,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              width: 160,
              borderRight: '1px solid var(--border-weak)',
              flexShrink: 0,
            }}
          >
            <DropdownSection label={groupLabel} />
            {groups.map((g) => {
              const active = g.value === hovGroup;
              return (
                <div
                  key={g.value}
                  onMouseEnter={() => setHovGroup(g.value)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: 'var(--space-1-5) var(--space-3)',
                    fontSize: 'var(--text-md)',
                    color: active
                      ? 'var(--color-primary-text)'
                      : 'var(--text-primary)',
                    background: active ? 'var(--action-selected)' : 'transparent',
                    cursor: 'pointer',
                    borderLeft: `2px solid ${active ? 'var(--color-primary)' : 'transparent'}`,
                  }}
                >
                  {g.label}
                  {active && <Icon name="chevron-right" size={10} />}
                </div>
              );
            })}
          </div>

          <div style={{ flex: 1 }}>
            <DropdownSection label={itemLabel} />
            {hovItems.map((item) => {
              const sel = hovGroup === value.group && item.value === value.item;
              return (
                <DropdownItem
                  key={item.value}
                  selected={sel}
                  onClick={() => {
                    onChange(hovGroup, item.value);
                    setOpen(false);
                  }}
                >
                  <span>{item.label}</span>
                  {sel && <Icon name="check" size={12} />}
                </DropdownItem>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
