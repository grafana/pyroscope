all: dashboard benchmark-dashboard benchmark-pr

benchmark-pr:
	jsonnet -J vendor benchmark-pr.jsonnet | tee gen/benchmark-pr.json

dashboard:
	jsonnet -J vendor dashboard.jsonnet | tee gen/dashboard.json

benchmark-dashboard:
	jsonnet -J vendor benchmark.jsonnet | tee gen/benchmark-dashboard.json

.PHONY: init
init:
	jb install
