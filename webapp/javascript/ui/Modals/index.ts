import Swal, { SweetAlertInput, SweetAlertOptions } from 'sweetalert2';

import styles from './Modal.module.css';

const defaultParams: Partial<SweetAlertOptions> = {
  showCancelButton: true,
  allowOutsideClick: true,
  backdrop: true,
  focusConfirm: false,
  customClass: {
    popup: styles.popup,
    title: styles.title,
    input: styles.input,
    confirmButton: styles.button,
    denyButton: styles.button,
    cancelButton: styles.button,
  },
};

export type ShowModalParams = {
  title: string;
  confirmButtonText: string;
  danger?: boolean;
  onConfirm: ShamefulAny;
  input?: SweetAlertInput;
  inputLabel?: string;
  inputPlaceholder?: string;
};

const ShowModal = ({
  title,
  confirmButtonText,
  danger,
  onConfirm,
  input,
  inputLabel,
  inputPlaceholder,
}: ShowModalParams) => {
  Swal.fire({
    title,
    confirmButtonText,
    input,
    inputLabel,
    inputPlaceholder,
    confirmButtonColor: danger ? '#dc3545' : '#0074d9',
    ...defaultParams,
  }).then(({ isConfirmed, value }) => {
    if (isConfirmed) {
      onConfirm(value);
    }
  });
};

export default ShowModal;
