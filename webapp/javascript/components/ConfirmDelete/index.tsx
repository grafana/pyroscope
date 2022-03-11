import ShowModal from '@ui/Modals';

function confirmDelete(object: string, onConfirm) {
  ShowModal({
    title: `Are you sure you want to delete ${object}?`,
    confirmButtonText: 'Delete',
    danger: true,
    onConfirm,
  });
}

export default confirmDelete;
