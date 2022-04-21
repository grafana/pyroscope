export PYROSCOPE_APPLICATION_NAME=fibonacci-java-lock-push
export PYROSCOPE_PROFILER_EVENT=lock
export PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040
java -javaagent:pyroscope.jar Main
