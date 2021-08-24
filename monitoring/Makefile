all: dashboard benchmark-dashboard

dashboard:
	jsonnet -J vendor dashboard.jsonnet | tee gen/dashboard.json

benchmark-dashboard:
	jsonnet -J vendor benchmark.jsonnet | tee gen/benchmark-dashboard.json

.PHONY: init
init:
	jb install
