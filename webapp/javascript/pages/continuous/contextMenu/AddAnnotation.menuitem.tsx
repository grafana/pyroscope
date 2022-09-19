import React, { useState } from 'react';
import { MenuItem, applyStatics } from '@webapp/ui/Menu';
import {
  Popover,
  PopoverBody,
  PopoverFooter,
  PopoverHeader,
} from '@webapp/ui/Popover';
import Button from '@webapp/ui/Button';
import { UncontrolledInputField } from '@webapp/ui/InputField';
import { Portal, PortalProps } from '@webapp/ui/Portal';

interface AddAnnotationProps {
  /** where to put the popover in the DOM */
  container: PortalProps['container'];

  /** where to put the popover */
  popoverAnchorPoint: {
    x: number;
    y: number;
  };
}

function AddAnnotation(props: AddAnnotationProps) {
  const { container, popoverAnchorPoint } = props;
  const [isPopoverOpen, setPopoverOpen] = useState(false);

  return (
    <>
      <MenuItem key="focus" onClick={() => setPopoverOpen(true)}>
        Add annotation
      </MenuItem>
      <Portal container={container}>
        <Popover
          anchorPoint={{ x: popoverAnchorPoint.x, y: popoverAnchorPoint.y }}
          isModalOpen={isPopoverOpen}
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
                label="Content"
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
      </Portal>
    </>
  );
}

// TODO: get rid of this in v3
// https://szhsin.github.io/react-menu-v2/docs#utils-apply-statics
// https://github.com/pyroscope-io/pyroscope/issues/1525
applyStatics(MenuItem)(AddAnnotation);

export default AddAnnotation;
