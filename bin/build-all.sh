#!/usr/bin/env bash
#!/bin/bash

set -e

BASE=$(dirname $0)
OUTDIR=${BASE}/../out
BINARYNAME=cloud-sidecars
CWD=$(pwd)
version=$(echo "${TRAVIS_BRANCH:-dev}" | sed 's/^v//')

sha256sumPath="sha256sum"

if [[ "$OSTYPE" == "darwin"* ]]; then
sha256sumPath="shasum -a 256"
fi

function build {
  local arch=$1; shift
  local os=$1; shift
  local ext=""

  if [ "${os}" == "windows" ]; then
      ext=".exe"
  fi

  cd ${CWD}
  echo "building ${BINARYNAME} (${os} ${arch})..."
  GOARCH=${arch} GOOS=${os} go build -ldflags="-s -w -X main.Version=${version}" -o $OUTDIR/${BINARYNAME}_${os}_${arch}${ext} github.com/orange-cloudfoundry/cloud-sidecars/cli/ || {
    echo >&2 "error: while building ${BINARYNAME} (${os} ${arch})"
    return 1
  }

  echo "zipping ${BINARYNAME} (${os} ${arch})..."
  cd $OUTDIR
  zip "${BINARYNAME}_${os}_${arch}.zip" "${BINARYNAME}_${os}_${arch}${ext}" || {
    echo >&2 "error: cannot zip file ${BINARYNAME}_${os}_${arch}${ext}"
    return 1
  }
  sum=$(${sha256sumPath} ${BINARYNAME}_${os}_${arch}${ext} | awk '{print $1}')
  echo "${BINARYNAME}_${os}_${arch}${ext} - ${sum}" >> sha256.txt
  cd ${CWD}
}

build amd64 windows
build amd64 linux
build amd64 darwin
