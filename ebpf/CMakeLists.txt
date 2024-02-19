cmake_minimum_required(VERSION 3.27)
project(pyrobpf C)

set(CMAKE_C_STANDARD 11)

add_library(pyrobpf bpf/profile.bpf.c)
target_include_directories(pyrobpf PRIVATE bpf bpf/vmlinux bpf/libbpf)
target_compile_definitions(pyrobpf PRIVATE -D__TARGET_ARCH_x86)

add_library(pyperf bpf/pyperf.bpf.c)
target_include_directories(pyperf PRIVATE bpf bpf/vmlinux bpf/libbpf)
target_compile_definitions(pyperf PRIVATE -D__TARGET_ARCH_x86)
