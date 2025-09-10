GL_USERNAME ?=
GL_PASSWORD ?=


.PHONY: remote
remote: push-pprof
	strace -f ./push-pprof \
		-url https://profiles-prod-008.grafana.net \
		-username $(GL_USERNAME) \
		-password  $(GL_PASSWORD) \
		2>strace.txt \
			| tee log.txt


.PHONY: local
local: push-pprof
	strace -f ./push-pprof \
		2>strace.txt \
			| tee log.txt


.PHONY:push-pprof
push-pprof:
	go build .


.PHONY: clean
clean:
	rm -rf keylog.txt strace.txt log.txt tracelog.txt
