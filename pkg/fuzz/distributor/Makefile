CXX ?= clang++
LDFLAGS ?= -L . -L /usr/local/lib -lresolv
# todo asan


.PHONY: all clean go-fuzz.a

all: fuzz


go-fuzz.a:
	CGO_ENABLED=1 go build -o $@ \
		-cover \
		-covermode=atomic \
		-buildmode c-archive \
		-buildvcs=false \
		-tags libfuzzer \
		-gcflags all=-d=libfuzzer \
		-gcflags syscall=-d=libfuzzer=0 \
		.


.PHONY: coverage
coverage:
	go tool covdata textfmt -i . -o result.txt
	go tool cover -html=./result.txt -o result.html
	go tool cover -func=./result.txt -o result_func.txt

# usage:
# GOCOVERDIR=. GOEXPERIMENT=coverageredesign ./fuzz ./corpus -max_total_time=60 -rss_limit_mb=0 -timeout=100000
fuzz: go-fuzz.a
	$(CXX) -o fuzz go-fuzz.a $(LDFLAGS) -fsanitize=fuzzer

clean:
	$(RM) go-fuzz.a go-fuzz.h