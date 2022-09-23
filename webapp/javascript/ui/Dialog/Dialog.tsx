/* eslint-disable react/jsx-props-no-spreading */
import React, { Ref, ReactNode } from 'react';
import ModalUnstyled from '@mui/base/ModalUnstyled';
import styles from './Dialog.module.css';

const Backdrop = React.forwardRef<
  HTMLDivElement,
  { open?: boolean; className: string }
>((props, ref) => {
  const { open, className, ...other } = props;
  return <div className={styles.backdrop} ref={ref} {...other} />;
});

interface DialogHeaderProps {
  children: ReactNode;
  closeable?: boolean;
  onClose?: () => void;
}
export const DialogHeader = React.forwardRef(
  (props: DialogHeaderProps, ref?: Ref<HTMLInputElement>) => {
    const { children, closeable, onClose } = props;
    return (
      <div className={styles.header} ref={ref}>
        {children}
        {closeable ? (
          <button
            aria-label="Close"
            className={styles.closeButton}
            onClick={onClose}
          />
        ) : null}
      </div>
    );
  }
);

interface DialogFooterProps {
  children: ReactNode;
}
export const DialogFooter = React.forwardRef(
  (props: DialogFooterProps, ref?: Ref<HTMLInputElement>) => {
    const { children } = props;
    return (
      <div className={styles.footer} ref={ref}>
        {children}
      </div>
    );
  }
);

interface DialogBodyProps {
  children: ReactNode;
}
export const DialogBody = React.forwardRef(
  (props: DialogBodyProps, ref?: Ref<HTMLInputElement>) => {
    const { children } = props;
    return (
      <div className={styles.body} ref={ref}>
        {children}
      </div>
    );
  }
);

type DialogProps = Exclude<
  React.ComponentProps<typeof ModalUnstyled>,
  'components'
>;
export function Dialog(props: DialogProps) {
  return (
    <ModalUnstyled
      {...props}
      components={{ Backdrop }}
      className={styles.modal}
    >
      <div className={styles.modalContainer}>{props.children}</div>
    </ModalUnstyled>
  );
}
