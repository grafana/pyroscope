all: dashboard benchmark-dashboard benchmark-pr gen/go-agent-benchmark-pr.json

benchmark-pr:
	jsonnet -J vendor benchmark-pr.jsonnet > gen/benchmark-pr.json

gen/go-agent-benchmark-pr.json: go-agent-benchmark-pr.jsonnet
	jsonnet -J vendor go-agent-benchmark-pr.jsonnet > gen/go-agent-benchmark-pr.json

dashboard:
	jsonnet -J vendor dashboard.jsonnet > gen/dashboard.json

benchmark-dashboard:
	jsonnet -J vendor benchmark.jsonnet > gen/benchmark-dashboard.json

.PHONY: init
init:
	jb install
