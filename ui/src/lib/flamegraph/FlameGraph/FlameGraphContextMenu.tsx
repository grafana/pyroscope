import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

import { Icon, type IconType } from '@components/core/Icon';

import './FlameGraphContextMenu.css';

import { type ClickedItemData, type SelectedView } from '../types';

import { type CollapseConfig, type DataFrame, type FlameGraphDataContainer } from './dataTransform';

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
      className="fg-ctx-menu"
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
      className="fg-ctx-item"
      onClick={onClick}
    >
      {icon && <Icon name={icon} size={14} />}
      <span>{label}</span>
    </button>
  );
}

function MenuGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className="fg-ctx-group">
      <div className="fg-ctx-group-label">{label}</div>
      {children}
    </div>
  );
}

export default FlameGraphContextMenu;
