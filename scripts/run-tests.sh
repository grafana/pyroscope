#!/usr/bin/env bash



for run in {1..10}; do
  res=$(yarn cy:ss-check)
  echo "$run statusCode=$?"

  if [ $? -ne 0 ]; then
    echo $res
  fi

done
