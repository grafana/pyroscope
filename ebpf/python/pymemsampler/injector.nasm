BITS 64

;    void (*free)(void *ctx, void *ptr);
  push 0xdead00
  ret
  push rdi
  push rsi



  mov rax, qword [free_ptr]
  mov qword [free_ptr_ptr], rax


  lea rdi, [rel fpath]
  mov rsi, 2 ; RTLD_NOW
  call qword [dlopen_ptr]
  pop rsi
  pop rdi
  call qword [free_ptr]
  ret

  align 16

dlopen_ptr: dq 0
free_ptr: dq 0
free_ptr_ptr: dq 0
fpath: