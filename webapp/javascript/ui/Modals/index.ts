import Swal, { SweetAlertInput, SweetAlertOptions } from 'sweetalert2';

const defaultParams: Partial<SweetAlertOptions> = {
  showCancelButton: true,
  allowOutsideClick: true,
  backdrop: true,
};

export type ShowModalParams = {
  title: string;
  confirmButtonText: string;
  danger?: boolean;
  onConfirm: ShamefulAny;
  input?: SweetAlertInput;
  inputLabel?: string;
  inputPlaceholder?: string;
  passResultValueToConfirmHandler?: boolean;
};

const ShowModal = ({
  title,
  confirmButtonText,
  danger,
  onConfirm,
  input,
  inputLabel,
  inputPlaceholder,
  passResultValueToConfirmHandler,
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
      if (passResultValueToConfirmHandler) {
        onConfirm(value);
        return;
      }
      onConfirm();
    }
  });
};

export default ShowModal;
