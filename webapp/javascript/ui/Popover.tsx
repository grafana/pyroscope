import React, { SetStateAction, Dispatch, ReactNode } from 'react';
import classnames from 'classnames';
import OutsideClickHandler from 'react-outside-click-handler';
import styles from './Popover.module.scss';

export interface ModalWithToggleProps {
  isModalOpen: boolean;
  setModalOpenStatus: Dispatch<SetStateAction<boolean>>;
  customHandleOutsideClick?: (e: MouseEvent) => void;
  children: ReactNode;
  modalClassName?: string;
  modalHeight?: string;

  // TODO(eh-am): type
  anchor?: React.MutableRefObject<HTMLElement>;
}

export function Popover({
  isModalOpen,
  setModalOpenStatus,
  modalClassName,
  children,
  anchor,
}: ModalWithToggleProps) {
  const anchorRect = anchor?.current?.getBoundingClientRect();
  console.log(anchor);
  console.log(anchor?.current);
  console.log(anchor?.current?.getBoundingClientRect());

  return (
    <div className={styles.container}>
      {isModalOpen && (
        <div className={classnames(styles.modal, modalClassName)}>
          {children}
        </div>
      )}
    </div>
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
