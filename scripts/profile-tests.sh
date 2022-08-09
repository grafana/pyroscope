#!/usr/bin/env bash

# generate a single pprof file for go tests
# this is only used by upload to flamegraph.com github action

#tmpDir=$(mktemp -d)
tmpDir=profiles
mkdir -p "$tmpDir"

if test $# -eq 0; then
  printf '<%s>\n' "$@";
  exit
fi

i=0
for package in $(go list -f '{{ .Dir }}' $@); do
  echo "Running tests for package $package"
  go test "$package" -cpuprofile="$tmpDir/$i.cpu"
  echo "Written to $tmpDir/$i.cpu"
  i=$((i+1))
done


#pprof-merge $tmpDir/*.cpu
