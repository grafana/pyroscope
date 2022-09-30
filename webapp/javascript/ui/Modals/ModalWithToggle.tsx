import React, { SetStateAction, Dispatch, ReactNode } from 'react';
import classnames from 'classnames';
import OutsideClickHandler from 'react-outside-click-handler';
import styles from './ModalWithToggle.module.scss';

export interface ModalWithToggleProps {
  isModalOpen: boolean;
  setModalOpenStatus: Dispatch<SetStateAction<boolean>>;
  customHandleOutsideClick?: (e: MouseEvent) => void;
  toggleText: string;
  headerEl: string | ReactNode;
  leftSideEl: ReactNode;
  rightSideEl: ReactNode;
  footerEl?: ReactNode;
  noDataEl?: ReactNode;
  modalClassName?: string;
  modalHeight?: string;
}

function ModalWithToggle({
  isModalOpen,
  setModalOpenStatus,
  customHandleOutsideClick,
  toggleText,
  headerEl,
  leftSideEl,
  rightSideEl,
  footerEl,
  noDataEl,
  modalClassName,
  modalHeight,
}: ModalWithToggleProps) {
  const handleOutsideClick = () => {
    setModalOpenStatus(false);
  };

  return (
    <div data-testid="modal-with-toggle" className={styles.container}>
      <OutsideClickHandler
        onOutsideClick={customHandleOutsideClick || handleOutsideClick}
      >
        <button
          id="modal-toggler"
          type="button"
          data-testid="toggler"
          className={styles.toggle}
          onClick={() => setModalOpenStatus((v) => !v)}
        >
          {toggleText}
        </button>
        {isModalOpen && (
          <div
            className={classnames(styles.modal, modalClassName)}
            data-testid="modal"
          >
            <div className={styles.modalHeader} data-testid="modal-header">
              {headerEl}
            </div>
            <div className={styles.modalBody} data-testid="modal-body">
              {noDataEl || (
                <>
                  <div
                    className={styles.side}
                    style={{ ...(modalHeight && { height: modalHeight }) }}
                  >
                    {leftSideEl}
                  </div>
                  <div
                    className={styles.side}
                    style={{ ...(modalHeight && { height: modalHeight }) }}
                  >
                    {rightSideEl}
                  </div>
                </>
              )}
            </div>
            <div className={styles.modalFooter} data-testid="modal-footer">
              {footerEl}
            </div>
          </div>
        )}
      </OutsideClickHandler>
    </div>
  );
}

export default ModalWithToggle;
