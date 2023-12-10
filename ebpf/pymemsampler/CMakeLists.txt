cmake_minimum_required(VERSION 3.26)
project(pymemsampler C)

set(CMAKE_C_STANDARD 23)

add_library(pymemsampler SHARED library.c)
