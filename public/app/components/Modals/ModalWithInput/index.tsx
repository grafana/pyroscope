import ShowModal, { ShowModalParams } from '@pyroscope/ui/Modals';

type ModalWithInputParams = Pick<
  ShowModalParams,
  | 'title'
  | 'input'
  | 'inputPlaceholder'
  | 'confirmButtonText'
  | 'onConfirm'
  | 'inputValue'
  | 'type'
  | 'validationMessage'
>;

async function showModalWithInput(params: ModalWithInputParams) {
  return ShowModal({
    ...params,
  });
}

export default showModalWithInput;
