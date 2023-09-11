/* eslint-disable react/jsx-props-no-spreading */
import React, { Dispatch, SetStateAction } from 'react';
import { Popover, PopoverBody, PopoverFooter } from '@pyroscope/ui/Popover';
import Button from '@pyroscope/ui/Button';
import { Portal } from '@pyroscope/ui/Portal';
import TextField from '@pyroscope/ui/Form/TextField';
import { AddAnnotationProps } from './AddAnnotation.menuitem';
import { useAnnotationForm } from './useAnnotationForm';

interface AnnotationInfoProps {
  /** where to position the popover */
  popoverAnchorPoint: AddAnnotationProps['popoverAnchorPoint'];
  timestamp: AddAnnotationProps['timestamp'];
  timezone: AddAnnotationProps['timezone'];
  value: { content: string; timestamp: number };
  isOpen: boolean;
  onClose: () => void;
  popoverClassname?: string;
}

const AnnotationInfo = ({
  popoverAnchorPoint,
  value,
  timezone,
  isOpen,
  onClose,
  popoverClassname,
}: AnnotationInfoProps) => {
  const { register, errors } = useAnnotationForm({ value, timezone });

  return (
    <Portal>
      <Popover
        anchorPoint={{ x: popoverAnchorPoint.x, y: popoverAnchorPoint.y }}
        isModalOpen={isOpen}
        setModalOpenStatus={onClose as Dispatch<SetStateAction<boolean>>}
        className={popoverClassname}
      >
        <PopoverBody>
          <form id="annotation-form" name="annotation-form">
            <TextField
              {...register('content')}
              label="Description"
              errorMessage={errors.content?.message}
              readOnly
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
          <Button onClick={onClose} kind="secondary" form="annotation-form">
            Close
          </Button>
        </PopoverFooter>
      </Popover>
    </Portal>
  );
};

export default AnnotationInfo;
