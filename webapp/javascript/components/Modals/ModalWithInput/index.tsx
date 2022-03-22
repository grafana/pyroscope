import ShowModal, { ShowModalParams } from '@ui/Modals';

type ModalWithInputParams = Pick<
  ShowModalParams,
  | 'title'
  | 'input'
  | 'inputLabel'
  | 'inputPlaceholder'
  | 'confirmButtonText'
  | 'onConfirm'
  | 'passResultValueToConfirmHandler'
>;

function showModalWithInput(params: ModalWithInputParams) {
  ShowModal({
    ...params,
  });
}

export default showModalWithInput;
