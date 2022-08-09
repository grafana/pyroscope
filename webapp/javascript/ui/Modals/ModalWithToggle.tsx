import React, { useState, ReactNode } from 'react';
import OutsideClickHandler from 'react-outside-click-handler';

import styles from './ModalWithToggle.module.scss';

function ModalWithToggle({
  toggleText,
  handleOutClick,
  headerEl,
  leftSideEl,
  rightSideEl,
  footerEl,
}: {
  toggleText: string;
  handleOutClick?: () => void;
  headerEl: string | ReactNode;
  leftSideEl: ReactNode;
  rightSideEl: ReactNode;
  footerEl?: ReactNode;
}) {
  const [isOpen, setIsOpen] = useState(false);

  const toggleModal = () => {
    setIsOpen((v) => !v);
  };

  const handleOutsideClick = () => {
    toggleModal();
    if (handleOutClick) handleOutClick();
  };

  return (
    <div data-testid="modal-with-toggle" className={styles.container}>
      <button
        data-testid="toggle"
        className={styles.toggle}
        onClick={toggleModal}
      >
        {toggleText}
      </button>
      {isOpen && (
        <OutsideClickHandler onOutsideClick={handleOutsideClick}>
          <div data-testid="modal" className={styles.modal}>
            <div className={styles.modalHeader}>{headerEl}</div>
            <div className={styles.modalBody}>
              <div className={styles.side}>{leftSideEl}</div>
              <div className={styles.side}>{rightSideEl}</div>
            </div>
            <div className={styles.modalFooter}>{footerEl}</div>
          </div>
        </OutsideClickHandler>
      )}
    </div>
  );
}

export default ModalWithToggle;
