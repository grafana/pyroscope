import ShowModal, { ShowModalParams } from '@webapp/ui/Modals';

interface ConfirmDeleteProps {
  objectType?: string;
  objectName?: string;
  object?: string;
  warningMsg?: string;
  onConfirm: () => void;
  withConfirmationInput?: boolean;
}

function confirmDelete({
  objectName,
  objectType,
  object,
  onConfirm,
  withConfirmationInput,
  warningMsg,
}: ConfirmDeleteProps) {
  // eslint-disable-next-line @typescript-eslint/no-floating-promises
  const confirmationInputProps: Partial<ShowModalParams> = withConfirmationInput
    ? {
        input: 'text' as ShowModalParams['input'],
        inputLabel: `Please type ${objectType} name`,
        inputPlaceholder: objectName,
        inputValidator: (value) =>
          value === objectName ? null : 'Name does not match',
      }
    : {};

  object = object || `${objectName} ${objectType}`;
  // eslint-disable-next-line @typescript-eslint/no-floating-promises
  ShowModal({
    title: `Are you sure you want to delete ${object}? ${
      warningMsg ? `\n ${warningMsg}` : ''
    }`,
    confirmButtonText: 'Delete',
    type: 'danger',
    onConfirm,
    ...confirmationInputProps,
  });
}

export default confirmDelete;
