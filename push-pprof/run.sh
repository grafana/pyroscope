set -ex

go run /home/korniltsev/pyroscope/push-pprof  -iteration-sleep 1s \
    -url ${GL_PROFILES_URL} \
		-username ${GL_USERNAME} \
		-password  ${GL_PASSWORD} \
			| tee log.txt
