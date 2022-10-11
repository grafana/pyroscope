/* eslint-disable react/jsx-props-no-spreading */
import React, { Dispatch, SetStateAction } from 'react';
import { MenuItem, applyStatics } from '@webapp/ui/Menu';
import { Popover, PopoverBody, PopoverFooter } from '@webapp/ui/Popover';
import Button from '@webapp/ui/Button';
import { Portal } from '@webapp/ui/Portal';
import TextField from '@webapp/ui/Form/TextField';
import { AddAnnotationProps } from './AddAnnotation.menuitem';
import { useAnnotationForm } from './useAnnotationForm';

interface AnnotationInfo {
  container: AddAnnotationProps['container'];
  popoverAnchorPoint: AddAnnotationProps['popoverAnchorPoint'];
  timestamp: AddAnnotationProps['timestamp'];
  timezone: AddAnnotationProps['timezone'];
  value: { content: string; timestamp: number };
  isOpen: boolean;
  setIsOpen: Dispatch<SetStateAction<boolean>>;
  popoverClassname?: string;
}

const AnnotationInfo = ({
  container,
  popoverAnchorPoint,
  value,
  timezone,
  isOpen,
  setIsOpen,
  popoverClassname,
}: AnnotationInfo) => {
  const { register, errors } = useAnnotationForm({ value, timezone });

  return (
    <Portal container={container}>
      <Popover
        anchorPoint={{ x: popoverAnchorPoint.x, y: popoverAnchorPoint.y }}
        isModalOpen={isOpen}
        setModalOpenStatus={setIsOpen}
        className={popoverClassname}
      >
        <PopoverBody>
          <form id="annotation-form" name="annotation-form">
            <TextField
              {...register('content')}
              label="Description"
              variant="light"
              errorMessage={errors.content?.message}
              readOnly
            />
            <TextField
              {...register('timestamp')}
              label="Time"
              type="text"
              readOnly
            />
          </form>
        </PopoverBody>
        <PopoverFooter>
          <Button
            onClick={() => setIsOpen(false)}
            kind="secondary"
            form="annotation-form"
          >
            Close
          </Button>
        </PopoverFooter>
      </Popover>
    </Portal>
  );
};

applyStatics(MenuItem)(AnnotationInfo);

export default AnnotationInfo;
