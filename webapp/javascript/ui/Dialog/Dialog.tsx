/* eslint-disable react/jsx-props-no-spreading */
import React, { Ref, ReactNode } from 'react';
import ModalUnstyled from '@mui/base/ModalUnstyled';
import Button from '@webapp/ui/Button';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import styles from './Dialog.module.css';

const Backdrop = React.forwardRef<
  HTMLDivElement,
  { open?: boolean; className: string }
>((props, ref) => {
  const { open, className, ...other } = props;
  return <div className={styles.backdrop} ref={ref} {...other} />;
});

type DialogHeaderProps = { children: ReactNode } & (
  | { closeable: true; onClose: () => void }
  | { closeable?: false }
);
export const DialogHeader = React.forwardRef(
  (props: DialogHeaderProps, ref?: Ref<HTMLInputElement>) => {
    const { children, closeable } = props;
    return (
      <div className={styles.header} ref={ref}>
        {children}
        {closeable ? (
          <Button
            aria-label="Close"
            icon={faTimes}
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

export default function DialogActions({
  children,
}: {
  children: React.ReactNode;
}) {
  return <div className={styles.dialogActions}>{children}</div>;
}
