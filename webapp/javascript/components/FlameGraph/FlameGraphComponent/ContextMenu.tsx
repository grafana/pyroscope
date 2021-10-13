import React from 'react';
import {
  ControlledMenu,
  useMenuState,
  MenuItem,
  SubMenu,
} from '@szhsin/react-menu';

// even though the library support many different types
// we only support these
type SupportedItems = typeof MenuItem | typeof SubMenu;

type xyToMenuItems = (x: number, y: number) => SupportedItems[];

export interface ContextMenuProps {
  canvasRef: React.RefObject<HTMLCanvasElement>;

  // The menu should be built dynamically
  // Based on the cell's contents
  xyToMenuItems: xyToMenuItems;
}

export default function ContextMenu(props: ContextMenuProps) {
  const { toggleMenu, openMenu, closeMenu, ...menuProps } = useMenuState(false);
  const [anchorPoint, setAnchorPoint] = React.useState({ x: 0, y: 0 });
  const { canvasRef } = props;
  const [menuItems, setMenuItems] = React.useState<SupportedItems[]>([]);

  const onContextMenu = (e: MouseEvent) => {
    e.preventDefault();

    const items = props.xyToMenuItems(e.clientX, e.clientY);
    setMenuItems(items);

    // TODO
    // if the menu becomes too large, it may overflow to outside the screen
    const x = e.clientX;
    const y = e.clientY + 20;

    setAnchorPoint({ x, y });
    openMenu();
  };

  React.useEffect(() => {
    closeMenu();

    // use closure to "cache" the current canvas reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const canvasEl = canvasRef.current;
    if (!canvasEl) {
      return () => {};
    }

    // watch for mouse events on the bar
    canvasEl.addEventListener('contextmenu', onContextMenu);

    return () => {
      canvasEl.removeEventListener('contextmenu', onContextMenu);
    };
  }, []);
  return (
    <ControlledMenu
      menuItemFocus={menuProps.menuItemFocus}
      isMounted={menuProps.isMounted}
      isOpen={menuProps.isOpen}
      anchorPoint={anchorPoint}
      onClose={closeMenu}
    >
      {menuItems}
    </ControlledMenu>
  );
}
