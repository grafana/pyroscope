import React, { useEffect, useState } from 'react';
import { useAppDispatch } from '@webapp/redux/hooks';
import {
  addAnnotation,
  fetchSingleView,
} from '@webapp/redux/reducers/continuous';
import { MenuItem, ControlledMenu } from '@szhsin/react-menu';
import Popover, {
  PopoverHeader,
  PopoverBody,
  PopoverFooter,
} from '@webapp/ui/Popover';
import Button from '@webapp/ui/Button';
import InputField from '@webapp/ui/InputField';

interface ContextMenuProps {
  x: number;
  y: number;

  /** timestamp of the clicked item */
  timestamp: number;
}

function ContextMenu(props: ContextMenuProps) {
  const { x, y, timestamp } = props;
  const [isOpen, setOpen] = useState(false);
  const dispatch = useAppDispatch();

  useEffect(() => {
    setOpen(true);
  }, []);

  const [isModalOpen, setModalOpen] = useState(false);

  // TODO(eh-am): handle out of bounds positioning
  const popoverPosition = {
    left: `${x}px`,
    top: `${y}px`,
    position: 'absolute' as const,
  };
  return (
    <>
      <ControlledMenu
        isOpen={isOpen}
        anchorPoint={{ x, y }}
        onClose={() => setOpen(false)}
      >
        <MenuItem key="focus" onClick={() => setModalOpen(true)}>
          Add annotation
        </MenuItem>
      </ControlledMenu>
      <div style={popoverPosition}>
        <Popover isModalOpen={isModalOpen} setModalOpenStatus={setModalOpen}>
          <PopoverHeader>Add annotation</PopoverHeader>
          <PopoverBody>
            <form
              id="annotation-form"
              name="annotation-form"
              onSubmit={async (event) => {
                event.preventDefault();

                const newAnnotation = {
                  appName: 'myapp',
                  content: event.target.content.value,
                  timestamp,
                };
                await dispatch(addAnnotation(newAnnotation));
                await dispatch(fetchSingleView(null));
              }}
            >
              <InputField
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
      </div>
    </>
  );
}

export default ContextMenu;
