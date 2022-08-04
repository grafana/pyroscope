package ebpfspy

// todo check ebpf map sizes

// todo check if kthreads are reported and try to filter them

// potential optimizations:
//    - share elf/so symbols between SymbolCache if inode is the same
//    - DeleteKeyBatch, GetValue
