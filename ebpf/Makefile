
.phony: python/dwarfdump
python/dwarfdump:
	git submodule update --init --recursive
	go run cmd/python_dwarfdump/main.go $(shell find testdata -name libpy\*.so\*) > python/versions_gen.go
	go fmt python/versions_gen.go


.phony: bpf/gen
bpf/gen:
	go generate pyrobpf/gen.go
	go generate python/gen.go