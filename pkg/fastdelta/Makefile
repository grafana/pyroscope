bench:
	go test . -bench 'BenchmarkDelta/pprof' -count=10 | tee pprof.txt
	go test . -bench 'BenchmarkDelta/fastdelta' -count=10 | tee fastdelta.txt
	go test . -bench 'BenchmarkDelta/pprof' -count=10 -memprofilerate=1 | tee pprof.mem.txt
	go test . -bench 'BenchmarkDelta/fastdelta' -count=10 -memprofilerate=1 | tee fastdelta.mem.txt

	sed 's#/fastdelta/#/#g' < fastdelta.txt > fastdelta.1.txt
	awk '{print $$1, $$2, $$3, $$4, $$5, $$6}' < fastdelta.1.txt > fastdelta.2.txt
	sed 's#/pprof/#/#g' < pprof.txt > pprof.1.txt
	awk '{print $$1, $$2, $$3, $$4, $$5, $$6}' < pprof.1.txt > pprof.2.txt
	benchstat pprof.2.txt fastdelta.2.txt > cpu.txt

	sed 's#/fastdelta/#/#g' < fastdelta.mem.txt > fastdelta.1.mem.txt
	awk '{print $$1, $$2, $$7, $$8, $$9, $$10, $$11, $$12}' < fastdelta.1.mem.txt > fastdelta.2.mem.txt
	sed 's#/pprof/#/#g' < pprof.mem.txt > pprof.1.mem.txt
	awk '{print $$1, $$2, $$7, $$8, $$9, $$10, $$11, $$12}' < pprof.1.mem.txt > pprof.2.mem.txt
	benchstat pprof.2.mem.txt fastdelta.2.mem.txt > mem.txt

	cat cpu.txt mem.txt | tee benchstat.txt



