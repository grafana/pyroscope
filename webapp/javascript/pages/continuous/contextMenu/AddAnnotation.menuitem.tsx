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
import { NewAnnotation } from '@webapp/services/annotations';

interface AddAnnotationProps {
  /** where to put the popover in the DOM */
  container: PortalProps['container'];

  /** where to put the popover */
  popoverAnchorPoint: {
    x: number;
    y: number;
  };

  onCreateAnnotation: (content: NewAnnotation['content']) => Promise<unknown>;
}

function AddAnnotation(props: AddAnnotationProps) {
  const { container, popoverAnchorPoint, onCreateAnnotation } = props;
  const [isPopoverOpen, setPopoverOpen] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

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
              onSubmit={(event) => {
                event.preventDefault();

                setIsSaving(true);

                // TODO(eh-am): validation
                // Keep popover open if there has been an error
                // TODO(eh-am): clicking on the notification will close this
                onCreateAnnotation(event.target.content.value as string)
                  .then(() => {
                    // TODO(eh-am): this triggers the following warning
                    // Warning: Can't perform a React state update on an unmounted component. This is a no-op, but it indicates a memory leak in your application. To fix, cancel all subscriptions and asynchronous tasks in a useEffect cleanup function.
                    setPopoverOpen(false);
                  })
                  .catch(() => {
                    setIsSaving(false);
                  });
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
