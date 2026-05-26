import { css } from '@emotion/css';
import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

import { type DataFrame } from '@grafana/data';

import { Icon, type IconType } from '@components/core/Icon';

import { type ClickedItemData, type SelectedView } from '../types';

import { type CollapseConfig, type FlameGraphDataContainer } from './dataTransform';

export type GetExtraContextMenuButtonsFunction = (
  clickedItemData: ClickedItemData,
  data: DataFrame,
  state: {
    selectedView?: SelectedView;
    search: string;
    collapseConfig?: CollapseConfig;
  }
) => ExtraContextMenuButton[];

export type ExtraContextMenuButton = {
  label: string;
  icon: IconType;
  onClick: () => void;
};

type Props = {
  data: FlameGraphDataContainer;
  itemData: ClickedItemData;
  onMenuItemClick: () => void;
  onItemFocus: () => void;
  onSandwich: () => void;
  onExpandGroup: () => void;
  onCollapseGroup: () => void;
  onExpandAllGroups: () => void;
  onCollapseAllGroups: () => void;
  getExtraContextMenuButtons?: GetExtraContextMenuButtonsFunction;
  collapseConfig?: CollapseConfig;
  collapsing?: boolean;
  allGroupsCollapsed?: boolean;
  allGroupsExpanded?: boolean;
  selectedView?: SelectedView;
  search: string;
};

const FlameGraphContextMenu = ({
  data,
  itemData,
  onMenuItemClick,
  onItemFocus,
  onSandwich,
  collapseConfig,
  onExpandGroup,
  onCollapseGroup,
  onExpandAllGroups,
  onCollapseAllGroups,
  getExtraContextMenuButtons,
  collapsing,
  allGroupsExpanded,
  allGroupsCollapsed,
  selectedView,
  search,
}: Props) => {
  const extraButtons =
    getExtraContextMenuButtons?.(itemData, data.data, {
      selectedView,
      search,
      collapseConfig,
    }) || [];

  return (
    <ContextMenu
      x={itemData.posX + 10}
      y={itemData.posY}
      onClose={onMenuItemClick}
      testId="contextMenu"
    >
      <MenuItem
        label="Focus block"
        icon="eye"
        onClick={() => {
          onItemFocus();
          onMenuItemClick();
        }}
      />
      <MenuItem
        label="Copy function name"
        icon="copy"
        onClick={() => {
          navigator.clipboard.writeText(itemData.label).then(() => {
            onMenuItemClick();
          });
        }}
      />
      <MenuItem
        label="Sandwich view"
        icon="sandwich"
        onClick={() => {
          onSandwich();
          onMenuItemClick();
        }}
      />
      {extraButtons.map(({ label, icon, onClick }) => (
        <MenuItem key={label} label={label} icon={icon} onClick={onClick} />
      ))}
      {collapsing && (
        <MenuGroup label="Grouping">
          {collapseConfig ? (
            collapseConfig.collapsed ? (
              <MenuItem
                label="Expand group"
                icon="angle-double-down"
                onClick={() => {
                  onExpandGroup();
                  onMenuItemClick();
                }}
              />
            ) : (
              <MenuItem
                label="Collapse group"
                icon="angle-double-up"
                onClick={() => {
                  onCollapseGroup();
                  onMenuItemClick();
                }}
              />
            )
          ) : null}
          {!allGroupsExpanded && (
            <MenuItem
              label="Expand all groups"
              icon="angle-double-down"
              onClick={() => {
                onExpandAllGroups();
                onMenuItemClick();
              }}
            />
          )}
          {!allGroupsCollapsed && (
            <MenuItem
              label="Collapse all groups"
              icon="angle-double-up"
              onClick={() => {
                onCollapseAllGroups();
                onMenuItemClick();
              }}
            />
          )}
        </MenuGroup>
      )}
    </ContextMenu>
  );
};

function ContextMenu({
  x,
  y,
  onClose,
  testId,
  children,
}: {
  x: number;
  y: number;
  onClose: () => void;
  testId?: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onDocClick = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
    };
  }, [onClose]);

  // Keep the menu inside the viewport
  const maxLeft = typeof window !== 'undefined' ? window.innerWidth - 220 : x;
  const maxTop = typeof window !== 'undefined' ? window.innerHeight - 280 : y;
  const left = Math.min(x, maxLeft);
  const top = Math.min(y, Math.max(0, maxTop));

  return createPortal(
    <div
      ref={ref}
      data-testid={testId}
      role="menu"
      className={styles.menu}
      style={{ left, top }}
    >
      {children}
    </div>,
    document.body
  );
}

function MenuItem({ label, icon, onClick }: { label: string; icon?: IconType; onClick: () => void }) {
  return (
    <button
      type="button"
      role="menuitem"
      className={styles.item}
      onClick={onClick}
    >
      {icon && <Icon name={icon} size={14} />}
      <span>{label}</span>
    </button>
  );
}

function MenuGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className={styles.group}>
      <div className={styles.groupLabel}>{label}</div>
      {children}
    </div>
  );
}

const styles = {
  menu: css({
    position: 'fixed',
    zIndex: 1000,
    background: 'var(--bg-elevated)',
    border: '1px solid var(--border-medium)',
    borderRadius: 'var(--radius-md)',
    boxShadow: 'var(--shadow-md)',
    padding: '4px 0',
    minWidth: 200,
    color: 'var(--text-primary)',
    fontSize: 'var(--text-sm)',
  }),
  item: css({
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    width: '100%',
    padding: '6px 12px',
    background: 'transparent',
    color: 'inherit',
    border: 'none',
    cursor: 'pointer',
    textAlign: 'left',
    '&:hover': {
      background: 'var(--action-hover)',
    },
  }),
  group: css({
    borderTop: '1px solid var(--border-weak)',
    marginTop: 4,
    paddingTop: 4,
  }),
  groupLabel: css({
    padding: '4px 12px',
    color: 'var(--text-secondary)',
    fontSize: 'var(--text-xs)',
    textTransform: 'uppercase',
    letterSpacing: '0.04em',
  }),
};

export default FlameGraphContextMenu;
