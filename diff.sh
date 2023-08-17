set -ex
export GO_GCFLAGS=-gcflags="-m=2  -d=pgodebug=3 -d=pgoinline=3"
make go/bin-debug 2> build-log-nopgo.txt
export GO_GCFLAGS="-gcflags=-m=2 -pgoprofile=ingester-3.pb.gz -d=pgodebug=3 -d=pgoinline=3"
make go/bin-debug 2> build-log-pgo.txt


cat build-log-nopgo.txt| grep -iE " inline" > inlines-no-pgo.txt
cat build-log-pgo.txt| grep -iE " inline" > inlines-pgo.txt
diff inlines-no-pgo.txt inlines-pgo.txt     > inline.diff
