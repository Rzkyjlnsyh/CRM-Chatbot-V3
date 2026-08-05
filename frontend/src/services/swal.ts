import Swal from 'sweetalert2';

const swal = Swal.mixin({
  confirmButtonColor: '#25D366',
  cancelButtonColor: '#757575',
  denyButtonColor: '#e53935',
});

/** Konfirmasi ya/tidak. Return true kalau user klik Ya. */
export async function swalConfirm(title: string, text?: string): Promise<boolean> {
  const result = await swal.fire({
    title,
    text,
    icon: 'question',
    showCancelButton: true,
    confirmButtonText: 'Ya',
    cancelButtonText: 'Batal',
  });
  return result.isConfirmed;
}

/** Prompt input teks. Return string atau null kalau batal. */
export async function swalPrompt(title: string, placeholder?: string): Promise<string | null> {
  const result = await swal.fire({
    title,
    input: 'text',
    inputPlaceholder: placeholder,
    showCancelButton: true,
    confirmButtonText: 'OK',
    cancelButtonText: 'Batal',
  });
  return result.isConfirmed ? (result.value || '') : null;
}

/** Alert info/error. `text` opsional untuk detail di bawah judul. */
export async function swalAlert(title: string, icon: 'success' | 'error' | 'warning' | 'info' = 'info', text?: string) {
  await swal.fire({ title, text, icon, confirmButtonText: 'OK' });
}

/** Toast kecil pojok kanan atas — tidak mengganggu. */
export function swalToast(title: string, icon: 'success' | 'error' | 'warning' | 'info' = 'success') {
  Swal.fire({
    title,
    icon,
    toast: true,
    position: 'top-end',
    showConfirmButton: false,
    timer: 2500,
    timerProgressBar: true,
  });
}
