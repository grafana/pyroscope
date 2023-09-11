/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { MenuItem } from '@pyroscope/ui/Menu';
import {
  Popover,
  PopoverBody,
  PopoverFooter,
  PopoverHeader,
} from '@pyroscope/ui/Popover';
import Button from '@pyroscope/ui/Button';
import { Portal, PortalProps } from '@pyroscope/ui/Portal';
import { NewAnnotation } from '@pyroscope/services/annotations';
import TextField from '@pyroscope/ui/Form/TextField';
import { useAnnotationForm } from './useAnnotationForm';
import styles from './AddAnnotation.menuitem.module.css';

export interface AddAnnotationProps {
  /** where to put the popover in the DOM */
  container: PortalProps['container'];

  /** where to position the popover */
  popoverAnchorPoint: {
    x: number;
    y: number;
  };

  onCreateAnnotation: (content: NewAnnotation['content']) => void;
  timestamp: number;
  timezone: 'browser' | 'utc';
}

function AddAnnotation(props: AddAnnotationProps) {
  const {
    container,
    popoverAnchorPoint,
    onCreateAnnotation,
    timestamp,
    timezone,
  } = props;
  const [isPopoverOpen, setPopoverOpen] = useState(false);
  const { register, handleSubmit, errors, setFocus } = useAnnotationForm({
    timezone,
    value: { timestamp },
  });

  // Focus on the only input
  React.useEffect(() => {
    if (isPopoverOpen) {
      setFocus('content');
    }
  }, [setFocus, isPopoverOpen]);

  const popoverContent = isPopoverOpen ? (
    <>
      <PopoverHeader>Add annotation</PopoverHeader>
      <PopoverBody>
        <form
          id="annotation-form"
          name="annotation-form"
          className={styles.form}
          onSubmit={handleSubmit((d) => {
            onCreateAnnotation(d.content as string);
          })}
        >
          <TextField
            {...register('content')}
            label="Description"
            variant="light"
            errorMessage={errors.content?.message}
            data-testid="annotation_content_input"
          />
          <TextField
            {...register('timestamp')}
            label="Time"
            type="text"
            readOnly
            data-testid="annotation_timestamp_input"
          />
        </form>
      </PopoverBody>
      <PopoverFooter>
        <Button type="submit" kind="secondary" form="annotation-form">
          Save
        </Button>
      </PopoverFooter>
    </>
  ) : null;

  return (
    <>
      <MenuItem key="focus" onClick={() => setPopoverOpen(true)}>
        Add annotation
      </MenuItem>
      <Portal container={container}>
        <Popover
          anchorPoint={{ x: popoverAnchorPoint.x, y: popoverAnchorPoint.y }}
          isModalOpen
          setModalOpenStatus={setPopoverOpen}
        >
          {popoverContent}
        </Popover>
      </Portal>
    </>
  );
}

export default AddAnnotation;
