/* eslint-disable react/jsx-props-no-spreading */
import React, { Ref, ReactNode } from 'react';
import ModalUnstyled from '@mui/base/ModalUnstyled';
import Button from '@webapp/ui/Button';
import cx from 'classnames';
import styles from './Dialog.module.css';

const Backdrop = React.forwardRef<
  HTMLDivElement,
  { open?: boolean; className: string }
>((props, ref) => {
  const { open, className, ...other } = props;
  return <div className={styles.backdrop} ref={ref} {...other} />;
});

type DialogHeaderProps = { children: ReactNode; className?: string } & (
  | { closeable: true; onClose: () => void }
  | { closeable?: false }
);
export const DialogHeader = React.forwardRef(
  (props: DialogHeaderProps, ref?: Ref<HTMLInputElement>) => {
    const { children, className, closeable } = props;
    return (
      <div className={cx(styles.header, className)} ref={ref}>
        {children}
        {closeable ? (
          <Button
            aria-label="Close"
            onClick={() => props.onClose()}
            noBox
            className={styles.closeButton}
          />
        ) : null}
      </div>
    );
  }
);

interface DialogFooterProps {
  children: ReactNode;
  className?: string;
}
export const DialogFooter = React.forwardRef(
  (props: DialogFooterProps, ref?: Ref<HTMLInputElement>) => {
    const { children, className } = props;
    return (
      <div className={cx(styles.footer, className)} ref={ref}>
        {children}
      </div>
    );
  }
);

interface DialogBodyProps {
  children: ReactNode;
  className?: string;
}
export const DialogBody = React.forwardRef(
  (props: DialogBodyProps, ref?: Ref<HTMLInputElement>) => {
    const { children, className } = props;
    return (
      <div className={cx(styles.body, className)} ref={ref}>
        {children}
      </div>
    );
  }
);

type DialogProps = Exclude<
  React.ComponentProps<typeof ModalUnstyled>,
  'components'
> & {
  className?: string;
  /** The header ID */
  ['aria-labelledby']: string;
};
export function Dialog(props: DialogProps) {
  const { className } = props;
  return (
    <ModalUnstyled
      {...props}
      components={{ Backdrop }}
      className={cx(styles.modal, className)}
    >
      <div
        aria-modal="true"
        aria-labelledby={props['aria-labelledby']}
        className={styles.modalContainer}
      >
        {props.children}
      </div>
    </ModalUnstyled>
  );
}

export function DialogActions({ children }: { children: React.ReactNode }) {
  return <div className={styles.dialogActions}>{children}</div>;
}
