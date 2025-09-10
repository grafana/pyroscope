GL_USERNAME ?=
GL_PASSWORD ?=
GL_URL ?=


.PHONY: remote
remote: push-pprof
	./push-pprof \
		-url $(GL_URL) \
		-username $(GL_USERNAME) \
		-password  $(GL_PASSWORD) \
			| tee log.txt


.PHONY: local
local: push-pprof
	 ./push-pprof \
			| tee log.txt


.PHONY:push-pprof
push-pprof:
	go build .


.PHONY: clean
clean:
	rm -rf keylog.txt strace.txt log.txt tracelog.txt
