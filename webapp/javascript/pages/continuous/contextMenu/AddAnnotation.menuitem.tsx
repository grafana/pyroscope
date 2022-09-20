/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { MenuItem, applyStatics } from '@webapp/ui/Menu';
import {
  Popover,
  PopoverBody,
  PopoverFooter,
  PopoverHeader,
} from '@webapp/ui/Popover';
import { format } from 'date-fns';
import { getUTCdate, timezoneToOffset } from '@webapp/util/formatDate';
import Button from '@webapp/ui/Button';
import { Portal, PortalProps } from '@webapp/ui/Portal';
import { NewAnnotation } from '@webapp/services/annotations';
import * as z from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import TextField from '@webapp/ui/Form/TextField';

interface AddAnnotationProps {
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

const newAnnotationFormSchema = z.object({
  content: z.string().min(1, { message: 'Required' }),
});

function AddAnnotation(props: AddAnnotationProps) {
  const {
    container,
    popoverAnchorPoint,
    onCreateAnnotation,
    timestamp,
    timezone,
  } = props;
  const [isPopoverOpen, setPopoverOpen] = useState(false);
  const {
    register,
    handleSubmit,
    formState: { errors },
    setFocus,
  } = useForm({
    resolver: zodResolver(newAnnotationFormSchema),
  });

  // Focus on the only input
  React.useEffect(() => {
    if (isPopoverOpen) {
      setFocus('content');
    }
  }, [setFocus, isPopoverOpen]);

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
              onSubmit={handleSubmit((d) => {
                onCreateAnnotation(d.content);
              })}
            >
              <TextField
                {...register('content')}
                label="Description"
                variant="light"
                errorMessage={errors.content?.message}
              />
              <TextField
                label="Time"
                type="text"
                readOnly
                value={format(
                  getUTCdate(
                    new Date(timestamp * 1000),
                    timezoneToOffset(timezone)
                  ),
                  'yyyy-MM-dd HH:mm'
                )}
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
