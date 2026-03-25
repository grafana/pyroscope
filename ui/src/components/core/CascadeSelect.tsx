import { useRef, useState } from 'react';
import { Button } from '@components/core/Button';
import { DropdownItem, DropdownSection } from '@components/core/Dropdown';
import { Icon } from '@components/core/Icon';
import { useClickOutside } from '@hooks/useClickOutside';
import './CascadeSelect.css';

type CascadeItem = { label: string; value: string } | { section: string };
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
      ?.items.find(
        (i): i is { label: string; value: string } =>
          !('section' in i) && i.value === value.item,
      )?.label ?? value.item;

  const hovItems = groups.find((g) => g.value === hovGroup)?.items ?? [];

  const buttonContent = loading ? (
    <>
      <span className="cascade-spinner" />
      Loading
    </>
  ) : noData ? (
    <span className="cascade-no-data">No data</span>
  ) : (
    <>
      {selectedGroupLabel} · {selectedItemLabel}
      <Icon name="angle-down" size={11} />
    </>
  );

  return (
    <div ref={ref} className="cascade-select" data-disabled={disabled}>
      <Button onClick={handleOpen} active={open}>
        {buttonContent}
      </Button>

      {open && (
        <div className="cascade-menu">
          <div className="cascade-groups">
            <DropdownSection label={groupLabel} />
            {groups.map((g) => {
              const active = g.value === hovGroup;
              return (
                <div
                  key={g.value}
                  onMouseEnter={() => setHovGroup(g.value)}
                  data-active={active}
                  className="cascade-group-row"
                >
                  {g.label}
                  {active && <Icon name="angle-right" size={10} />}
                </div>
              );
            })}
          </div>

          <div className="cascade-items">
            <DropdownSection label={itemLabel} />
            {hovItems.map((item, idx) => {
              if ('section' in item)
                return (
                  <DropdownSection
                    key={`section-${idx}`}
                    label={item.section}
                    subsection
                  />
                );
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
