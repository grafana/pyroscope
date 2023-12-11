cmake_minimum_required(VERSION 3.0)
project(pymemsampler C)

set(CMAKE_C_STANDARD 23)
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -Werror")

add_library(pymemsampler SHARED library.c)
