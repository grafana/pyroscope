/* eslint-disable react/jsx-props-no-spreading, import/no-extraneous-dependencies */
import React from 'react';
import { ControlledMenu, useMenuState } from '@pyroscope/ui/Menu';
import styles from './ContextMenu.module.scss';

type xyToMenuItems = (x: number, y: number) => JSX.Element[];

export interface ContextMenuProps {
  canvasRef: React.RefObject<HTMLCanvasElement>;

  /**
   * The menu is built dynamically
   * Based on the cell's contents
   * only MenuItem and SubMenu should be supported
   */
  xyToMenuItems: xyToMenuItems;

  onClose: () => void;
  onOpen: (x: number, y: number) => void;
}

export default function ContextMenu(props: ContextMenuProps) {
  const [menuProps, toggleMenu] = useMenuState({ transition: true });
  const [anchorPoint, setAnchorPoint] = React.useState({ x: 0, y: 0 });
  const { canvasRef } = props;
  const [menuItems, setMenuItems] = React.useState<JSX.Element[]>([]);
  const {
    xyToMenuItems,
    onClose: onCloseCallback,
    onOpen: onOpenCallback,
  } = props;

  const onClose = () => {
    toggleMenu(false);

    onCloseCallback();
  };

  React.useEffect(() => {
    toggleMenu(false);

    // use closure to "cache" the current canvas reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const canvasEl = canvasRef.current;
    if (!canvasEl) {
      return () => {};
    }

    const onContextMenu = (e: MouseEvent) => {
      e.preventDefault();

      const items = xyToMenuItems(e.offsetX, e.offsetY);
      setMenuItems(items);

      // TODO
      // if the menu becomes too large, it may overflow to outside the screen
      const x = e.clientX;
      const y = e.clientY + 20;

      setAnchorPoint({ x, y });
      toggleMenu(true);

      onOpenCallback(e.offsetX, e.offsetY);
    };

    // watch for mouse events on the bar
    canvasEl.addEventListener('contextmenu', onContextMenu);

    return () => {
      canvasEl.removeEventListener('contextmenu', onContextMenu);
    };
  }, [xyToMenuItems, canvasRef, onOpenCallback, toggleMenu]);

  return (
    <ControlledMenu
      {...menuProps}
      className={styles.dummy}
      anchorPoint={anchorPoint}
      onClose={onClose}
    >
      {menuItems}
    </ControlledMenu>
  );
}
