import ShowModal from '@ui/Modals';

function confirmDelete(object: string, onConfirm: () => void) {
  ShowModal({
    title: `Are you sure you want to delete ${object}?`,
    confirmButtonText: 'Delete',
    type: 'danger',
    onConfirm,
  });
}

export default confirmDelete;
