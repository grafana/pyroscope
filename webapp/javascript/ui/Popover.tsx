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
}

function Popover({
  isModalOpen,
  setModalOpenStatus,
  modalClassName,
  children,
}: ModalWithToggleProps) {
  return (
    <div data-testid="modal-with-toggle" className={styles.container}>
      {isModalOpen && (
        <OutsideClickHandler onOutsideClick={() => setModalOpenStatus(false)}>
          <div className={classnames(styles.modal, modalClassName)}>
            {children}
          </div>
        </OutsideClickHandler>
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

export default Popover;
