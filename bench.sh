#!/usr/bin/env bash
# shellcheck disable=SC2086

# This script is for benchmarking the current branch against main.
# We first check the dependencies
# Then git status, requiring us to start on a branch
# with all work committed. We enter a loop, bouncing between
# the two branches, running benchmarking groups alternately.
# Finally, output from all runs is merged, and a before/after
# diff is generated.

DEPENDENCY_CHECK() {
	DEP=$1
	WHICH_DEP=$(eval which $DEP)
	DEP_SRC=$2
	if [[ $WHICH_DEP == "" ]]; then
		# shellcheck disable=SC2162
		read -p "$DEP required, install now? [Y/n]: " Yn
		echo
	  if [[ $Yn == "n" ]]; then
	    echo "exiting" && exit 1
	  fi
		eval go install $DEP_SRC
		exit 0
	fi
	echo "found $DEP: $WHICH_DEP"
}

DEPENDENCY_CHECK "benchstat" "golang.org/x/perf/cmd/benchstat@latest"
DEPENDENCY_CHECK "pprof-merge" "github.com/rakyll/pprof-merge"

# This script MUST be run from the root directory
CURRENT_DIR=${PWD##*/}
if [[ "$CURRENT_DIR" != "pyroscope" ]]; then
	echo "please run script from the root directory"
	exit 1
fi

ITERATIONS="${1:-6}"

# check git status
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
GIT_REVISION=$(git rev-parse --short HEAD)
GIT_STATUS=$(git status --short)

if [[ "$GIT_BRANCH" == "main" ]]; then
	echo "on main, please checkout branch to benchmark"
	exit 1
else
	echo "on branch $GIT_BRANCH at $GIT_REVISION"
fi

if [[ "$GIT_STATUS" == "" ]]; then
	echo "all changes committed, ready to begin benchmarking"
else
	# TODO: support stashing changes
	echo "some changes not committed, please commit all work"
	exit 1
fi

# setup paths/filenames
BRANCH_TAG=${GIT_BRANCH//'/'/'_'}
BASE_DIR=$(realpath ./)
BRANCH_DIR=${BASE_DIR}/data/benchmarks_${BRANCH_TAG}
MAIN_DIR=${BASE_DIR}/data/benchmarks_main

# setup and clean up the files
if [ ! -d ./data ]
then
  mkdir data
fi

rm -rf $BRANCH_DIR
mkdir $BRANCH_DIR
rm -rf $MAIN_DIR
mkdir $MAIN_DIR

# begin benchmarking
# this interleaved loop technique is idiomatic, from docs:
# https://pkg.go.dev/golang.org/x/perf/cmd/benchstat#hdr-Tips
i=1
while [[ $i -le $ITERATIONS ]]
do
	set -e # exit on any error
	set -o pipefail # including when we're piping things
	echo "bench ${i} on ${GIT_BRANCH}"

	go test -bench=Ingester -run=XXX -short -benchmem \
		-memprofile=${BRANCH_DIR}/mem_${i}.prof \
		-cpuprofile=${BRANCH_DIR}/cpu_${i}.prof \
		./pkg/ingester | tee -a $BRANCH_DIR/bench.txt

	eval git checkout main
	echo "bench ${i} on main"
	go test -bench=Ingester -run=XXX -short -benchmem \
		-memprofile=${MAIN_DIR}/mem_${i}.prof \
		-cpuprofile=${MAIN_DIR}/cpu_${i}.prof \
		./pkg/ingester | tee -a $MAIN_DIR/bench.txt 

	eval git checkout $GIT_BRANCH
	echo "bench ${i} completed"
	((i = i + 1))
done

MERGE_PROFILES() {
	DIR=$1
	NAME=$2

	if [[ "$DIR" == "" || "$NAME" == "" ]]; then
		echo "parameter missing"
		exit 1
	fi

	pprof-merge ${DIR}/${NAME}_*
	rm ${DIR}/${NAME}_*
	mv merged.data ${DIR}/${NAME}.prof
}

MERGE_PROFILES $BRANCH_DIR "mem"
MERGE_PROFILES $BRANCH_DIR "cpu"
MERGE_PROFILES $MAIN_DIR "mem"
MERGE_PROFILES $MAIN_DIR "cpu"

# generate cpu delta
go tool pprof -top -base=${MAIN_DIR}/cpu.prof \
	${BRANCH_DIR}/cpu.prof | tee ${BRANCH_DIR}/cpu_delta.txt

# generate memory delta
go tool pprof -top -base=${MAIN_DIR}/mem.prof \
	${BRANCH_DIR}/mem.prof | tee ${BRANCH_DIR}/mem_delta.txt

# generate benchstat delta
cp ${MAIN_DIR}/bench.txt ${BRANCH_DIR}/main_bench.txt
cd ${BRANCH_DIR}
benchstat main_bench.txt bench.txt | tee benchstat.txt
