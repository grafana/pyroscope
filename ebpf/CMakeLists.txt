cmake_minimum_required(VERSION 3.27)
project(pyrobpf C)
# This is a dumb project to help using IDE/tools to develop bpf program
# CLion can load cmake project
# Cmake+ninja can also generate compile_commands.json which may used by clangd etc
set(CMAKE_C_STANDARD 11)

add_library(pyrobpf bpf/profile.bpf.c)
target_include_directories(pyrobpf PRIVATE bpf bpf/vmlinux bpf/libbpf)
target_compile_definitions(pyrobpf PRIVATE -D__TARGET_ARCH_x86)
target_compile_options(pyrobpf PRIVATE -nostdinc -nostdlib)

add_library(pyperf bpf/pyperf.bpf.c)
target_include_directories(pyperf PRIVATE bpf bpf/vmlinux bpf/libbpf)
target_compile_definitions(pyperf PRIVATE -D__TARGET_ARCH_x86)
target_compile_options(pyperf PRIVATE -nostdinc -nostdlib)
