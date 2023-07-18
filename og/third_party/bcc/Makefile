BCC_VERSION ?= b9554b585afe18540ba98dde5b667e5b4036f479

.PHONY: build-bcc
build-bcc:
	test -d src || git clone https://github.com/korniltsev/bcc src
	cd src && git checkout $(BCC_VERSION)
	test -d src/build && rm -rf src/build || echo bcc src/build dir does not exits
	mkdir src/build
	cd src/build \
		&& cmake ../build-syms  \
			-DCMAKE_C_COMPILER=clang \
			-DCMAKE_CXX_COMPILER=clang++ \
			-DCMAKE_BUILD_TYPE=Release \
			-DCMAKE_INSTALL_PREFIX=$(shell pwd)/lib \
        		&& make -j16 install

.PHONY: clean
clean:
	rm -rf src lib
