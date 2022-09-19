import React, { useEffect, useState } from 'react';
import { ControlledMenu } from '@webapp/ui/Menu';
import { ContextMenuProps } from '@webapp/components/TimelineChart/ContextMenu.plugin';
import AddAnnotationMenuItem from './AddAnnotation.menuitem';

function ContextMenu(props: ContextMenuProps) {
  const { click } = props;
  const [isOpen, setOpen] = useState(false);

  // https://github.com/szhsin/react-menu/issues/2#issuecomment-719166062
  useEffect(() => {
    setOpen(true);
  }, []);

  return (
    <>
      <ControlledMenu
        isOpen={isOpen}
        anchorPoint={{ x: click.pageX, y: click.pageY }}
        onClose={() => setOpen(false)}
      >
        <AddAnnotationMenuItem
          container={props.containerEl}
          popoverAnchorPoint={{ x: click.pageX, y: click.pageY }}
        />
      </ControlledMenu>
    </>
  );
}

export default ContextMenu;
