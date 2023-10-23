
.phony: python_dwarfdump
python_dwarfdump:
	git submodule update --init --recursive
	go run cmd/python_dwarfdump/main.go $(shell find testdata -name libpy\*.so\*) > python/versions_gen.go
	go fmt python/versions_gen.go

