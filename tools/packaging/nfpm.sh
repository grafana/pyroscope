#!/usr/bin/env bash

if [[ -z "${NFPM_SIGNING_KEY_FILE}" ]]; then
  echo "NFPM_SIGNING_KEY_FILE is not set"
  exit 1
fi
if [[ -z "${NFPM_PASSPHRASE}" ]]; then
  echo "NFPM_PASSPHRASE is not set"
  exit 1
fi

rm -rf dist/tmp && mkdir -p dist/tmp/packages

for name in pyroscope profilecli; do
  for arch in amd64 arm64; do
    config_path="dist/tmp/config-${name}-${arch}.json"
    suffix=""
    if [ $arch = "amd64" ]; then
      suffix="_v1"
    fi
    jsonnet -V "name=${name}" -V "arch=${arch}" -V "suffix=${suffix}" "tools/packaging/nfpm.jsonnet" >"${config_path}"
    nfpm package -f "${config_path}" -p rpm -t dist/
    nfpm package -f "${config_path}" -p deb -t dist/
  done
done

rm -rf dist/tmp
