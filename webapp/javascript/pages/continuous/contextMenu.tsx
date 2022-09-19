import React, { useEffect, useState } from 'react';
import { MenuItem, ControlledMenu } from '@szhsin/react-menu';
import { ContextMenuProps } from '@webapp/components/TimelineChart/ContextMenu.plugin';

function ContextMenu(props: ContextMenuProps) {
  const { click } = props;
  const [isOpen, setOpen] = useState(false);

  useEffect(() => {
    setOpen(true);
  }, []);

  const [isModalOpen, setModalOpen] = useState(false);

  return (
    <>
      <ControlledMenu
        isOpen={isOpen}
        anchorPoint={{ x: click.pageX, y: click.pageY }}
        onClose={() => setOpen(false)}
      >
        <MenuItem key="focus" onClick={() => setModalOpen(true)}>
          Add annotation
        </MenuItem>
      </ControlledMenu>
    </>
  );
}

export default ContextMenu;
