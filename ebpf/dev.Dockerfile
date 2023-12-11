FROM ubuntu:22.04

RUN apt-get update
RUN apt-get -y install clang
RUN apt-get -y install make
RUN apt-get -y install llvm


RUN apt-get -y install wget
RUN wget https://dl.google.com/go/go1.21.5.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin


RUN apt-get -y install cmake
RUN apt-get -y install nasm
RUN apt-get -y install gdb
