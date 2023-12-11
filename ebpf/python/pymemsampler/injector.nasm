BITS 64

;    void (*free)(void *ctx, void *ptr);
  db 0xcc

  push rdi ; align stack to 16 ; todo make it proper
  push rdi
  push rsi



  mov rax, qword [rel free_ptr]
  mov rbx, qword [rel free_ptr_ptr]
  mov [rbx], rax


  lea rdi, [rel fpath]
  mov rsi, 2 ; RTLD_NOW
  call qword [rel dlopen_ptr]
  pop rsi
  pop rdi
  call qword [rel free_ptr]
  pop rdi ; unalign stack to 16 ; todo make it proper
  ret

  align 16

dlopen_ptr: dq 0
free_ptr: dq 0
free_ptr_ptr: dq 0
fpath: