import React, { SetStateAction, Dispatch, ReactNode } from 'react';
import classnames from 'classnames';
import OutsideClickHandler from 'react-outside-click-handler';
import styles from './Popover.module.scss';

export interface PopoverProps {
  isModalOpen: boolean;
  setModalOpenStatus: Dispatch<SetStateAction<boolean>>;
  children: ReactNode;
  className?: string;

  /** where to position the popover on the page */
  anchorPoint: {
    x: number;
    y: number;
  };
}

export function Popover({
  isModalOpen,
  setModalOpenStatus,
  className,
  children,
  anchorPoint,
}: PopoverProps) {
  // TODO(eh-am): handle out of bounds positioning
  const popoverPosition = {
    left: `${anchorPoint.x}px`,
    top: `${anchorPoint.y}px`,
    position: 'absolute' as const,
  };

  return (
    <OutsideClickHandler onOutsideClick={() => setModalOpenStatus(false)}>
      <div className={styles.container} style={popoverPosition}>
        {isModalOpen && (
          <div className={classnames(styles.modal, className)}>{children}</div>
        )}
      </div>
    </OutsideClickHandler>
  );
}

interface PopoverMemberProps {
  children: ReactNode;
}

export function PopoverHeader({ children }: PopoverMemberProps) {
  return <div className={styles.modalHeader}>{children}</div>;
}

export function PopoverBody({ children }: PopoverMemberProps) {
  return <div className={styles.modalBody}>{children}</div>;
}

export function PopoverFooter({ children }: PopoverMemberProps) {
  return <div className={styles.modalFooter}>{children}</div>;
}
