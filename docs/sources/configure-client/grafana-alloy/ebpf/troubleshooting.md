---
title: "Troubleshoot eBPF installation"
menuTitle: "Troubleshoot"
description: "Troubleshoot Grafana eBPF installation."
weight: 40
---

# Troubleshoot eBPF installation

Learn how to troubleshoot and resolve eBPF installation issues.

## Profile interpreted languages

Profiling interpreted languages like Ruby, JavaScript, etc., isn't ideal using this implementation.
The JIT-compiled methods in these languages are typically not in ELF file format, demanding additional steps for
profiling. For instance, using perf-map-agent and enabling frame pointers for Java.

Interpreted methods display the interpreter function’s name rather than the actual function.

## Troubleshoot unknown symbols

Symbols are extracted from various sources, including:

* The `.symtab` and `.dynsym` sections in the ELF file.
* The `.symtab` and `.dynsym` sections in the debug ELF file.
* The `.gopclntab` section in Go language ELF files.

The search for debug files follows [gdb algorithm](https://sourceware.org/gdb/onlinedocs/gdb/Separate-Debug-Files.html).
For example, if the profiler wants to find the debug file
for `/lib/x86_64-linux-gnu/libc.so.6`
with a `.gnu_debuglink` set to `libc.so.6.debug` and a build ID `0123456789abcdef`. The following paths are examined:

* `/usr/lib/debug/.build-id/01/0123456789abcdef.debug`
* `/lib/x86_64-linux-gnu/libc.so.6.debug`
* `/lib/x86_64-linux-gnu/.debug/libc.so.6.debug`
* `/usr/lib/debug/lib/x86_64-linux-gnu/libc.so.6.debug`

### Deal with unknown symbols

Unknown symbols in the profiles you’ve collected indicate that the profiler couldn't access an ELF file associated with a given address in the trace.

This can occur for several reasons:

* The process has terminated, making the ELF file inaccessible.
* The ELF file is either corrupted or not recognized as an ELF file.
* There is no corresponding ELF file entry in `/proc/pid/maps` for the address in the stack trace.

### Address unresolved symbols

If you only see module names (e.g., `/lib/x86_64-linux-gnu/libc.so.6`) without corresponding function names, this
indicates that the symbols couldn't be mapped to their respective function names.

This can occur for several reasons:

* The binary has been stripped, leaving no `.symtab`, `.dynsym`, or `.gopclntab` sections in the ELF file.
* The debug file is missing or could not be located.

To fix this for your binaries, ensure that they are either not stripped or that you have separate
debug files available. You can achieve this by running:

```bash
objcopy --only-keep-debug elf elf.debug
strip elf -o elf.stripped
objcopy --add-gnu-debuglink=elf.debug elf.stripped elf.debuglink
```

For system libraries, ensure that debug symbols are installed. On Ubuntu, for example, you can install debug symbols
for `libc` by executing:

```bash
apt install libc6-dbg
```

### Understand flat stack traces

If your profiles show many shallow stack traces, typically 1-2 frames deep, your binary might have been compiled without frame pointers.

To compile your code with frame pointers, include the `-fno-omit-frame-pointer` flag in your compiler options.
