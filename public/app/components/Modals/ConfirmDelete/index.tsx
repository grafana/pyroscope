import ShowModal, { ShowModalParams } from '@pyroscope/ui/Modals';

interface ConfirmDeleteProps {
  objectType: string;
  objectName: string;
  onConfirm: () => void;
  warningMsg?: string;
  withConfirmationInput?: boolean;
}

function confirmDelete({
  objectName,
  objectType,
  onConfirm,
  withConfirmationInput,
  warningMsg,
}: ConfirmDeleteProps) {
  const confirmationInputProps: Partial<ShowModalParams> = withConfirmationInput
    ? {
        input: 'text' as ShowModalParams['input'],
        inputLabel: `To confirm deletion enter ${objectType} name below.`,
        inputPlaceholder: objectName,
        inputValidator: (value) =>
          value === objectName ? null : 'Name does not match',
      }
    : {};

  // eslint-disable-next-line @typescript-eslint/no-floating-promises
  ShowModal({
    title: `Delete ${objectType}`,
    html: `Are you sure you want to delete<br><strong>${objectName}</strong> ?${
      warningMsg ? `<br><br>${warningMsg}` : ''
    }`,
    confirmButtonText: 'Delete',
    type: 'danger',
    onConfirm,
    ...confirmationInputProps,
  });
}

export default confirmDelete;
