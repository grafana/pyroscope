import Swal, { SweetAlertOptions } from 'sweetalert2';

const defaultParams: Partial<SweetAlertOptions> = {
  showCancelButton: true,
  allowOutsideClick: true,
  backdrop: true,
};

type ShowModalParams = {
  title: string;
  confirmButtonText: string;
  danger: boolean;
  onConfirm: ShamefulAny;
};

const ShowModal = ({
  title,
  confirmButtonText,
  danger,
  onConfirm,
}: ShowModalParams) => {
  Swal.fire({
    title,
    confirmButtonText,
    confirmButtonColor: danger ? '#dc3545' : '#0074d9',
    ...defaultParams,
  }).then((result) => {
    if (result.isConfirmed) {
      onConfirm();
    }
  });
};

export default ShowModal;
