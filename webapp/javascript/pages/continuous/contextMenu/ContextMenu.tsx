import React, { useEffect, useState } from 'react';
import { ControlledMenu } from '@webapp/ui/Menu';
import { ContextMenuProps as PluginContextMenuProps } from '@webapp/components/TimelineChart/Annotations.plugin';

interface ContextMenuProps {
  /** position */
  position: PluginContextMenuProps['click'];

  /** must be not empty */
  children: React.ReactNode;
}

function ContextMenu(props: ContextMenuProps) {
  const { position, children } = props;
  const [isOpen, setOpen] = useState(false);

  // https://github.com/szhsin/react-menu/issues/2#issuecomment-719166062
  useEffect(() => {
    setOpen(true);
  }, []);

  return (
    <>
      <ControlledMenu
        isOpen={isOpen}
        anchorPoint={{ x: position.pageX, y: position.pageY }}
        onClose={() => setOpen(false)}
      >
        {children}
      </ControlledMenu>
    </>
  );
}

export default ContextMenu;
