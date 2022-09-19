import React, { useEffect, useState } from 'react';
import { MenuItem, ControlledMenu } from '@szhsin/react-menu';
import { ContextMenuProps } from '@webapp/components/TimelineChart/ContextMenu.plugin';
import {
  Popover,
  PopoverBody,
  PopoverFooter,
  PopoverHeader,
} from '@webapp/ui/Popover';
import Button from '@webapp/ui/Button';
import { UncontrolledInputField } from '@webapp/ui/InputField';

function ContextMenu(props: ContextMenuProps) {
  const { click } = props;
  const [isOpen, setOpen] = useState(false);

  // https://github.com/szhsin/react-menu/issues/2#issuecomment-719166062
  useEffect(() => {
    setOpen(true);
  }, []);

  const [isModalOpen, setPopoverOpen] = useState(false);

  return (
    <>
      <ControlledMenu
        isOpen={isOpen}
        anchorPoint={{ x: click.pageX, y: click.pageY }}
        onClose={() => setOpen(false)}
      >
        <MenuItem key="focus" onClick={() => setPopoverOpen(true)}>
          Add annotation
        </MenuItem>
      </ControlledMenu>
      <Popover
        anchorPoint={{ x: click.pageX, y: click.pageY }}
        isModalOpen={isModalOpen}
        setModalOpenStatus={setPopoverOpen}
      >
        <PopoverHeader>Add annotation</PopoverHeader>
        <PopoverBody>
          <form
            id="annotation-form"
            name="annotation-form"
            onSubmit={async (event) => {
              event.preventDefault();

              console.log('submitted form with value', {
                value: event.target.content.value,
              });

              // TODO
              // dispatch
              // wait for event to be handled
              // close modal
              setPopoverOpen(false);
            }}
          >
            <UncontrolledInputField
              type="text"
              label="Text"
              placeholder=""
              name="content"
              required
            />
          </form>
        </PopoverBody>
        <PopoverFooter>
          <Button type="submit" kind="secondary" form="annotation-form">
            Save
          </Button>
        </PopoverFooter>
      </Popover>
    </>
  );
}

export default ContextMenu;
