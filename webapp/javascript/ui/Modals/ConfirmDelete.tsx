import Swal from 'sweetalert2';

function confirmDelete(object: string, onConfirm) {
  Swal.fire({
    title: `Are you sure you want to delete ${object}?`,
    showCancelButton: true,
    confirmButtonText: 'Delete',
    backdrop: true,
    allowOutsideClick: true,
    confirmButtonColor: '#dc3545',
  }).then((result) => {
    if (result.isConfirmed) {
      onConfirm();
    }
  });
}

export default confirmDelete;
