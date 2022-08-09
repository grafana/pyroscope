package ebpfspy

// todo check ebpf map sizes

// todo check if kthreads are reported and try to filter them

// potential optimizations:
//    - share elf/so symbols between SymbolCache if inode is the same
//    - DeleteKeyBatch, GetValue

// maybe use flags for stacks:  bit 10 - if two different stacks hash into the same stackid - for stackids
//discard old
// maybe use map in map or map in array, to avoid clearing map by keys
// measure stacks.GetValue, sym.resolve separately
// try concurrent
// offload demangling to the server?
