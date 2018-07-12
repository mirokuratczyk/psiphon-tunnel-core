#!/usr/bin/env bash

# -x echos commands. 
# -e exits if a command returns an error.
set -x -e

# This script takes an optional second argument: 'private', if private plugins should
# be used. It should be omitted if private plugins are not desired.
if [[ $2 == "private" ]]; then
  FORCE_PRIVATE_PLUGINS=true
  echo "TRUE"
else
  FORCE_PRIVATE_PLUGINS=false
  echo "FALSE"
fi

# Modify this value as we use newer Go versions.
GO_VERSION_REQUIRED="1.9.6"

BASE_DIR=$(cd "$(dirname "$0")" ; pwd -P)
cd ${BASE_DIR}

# The location of the final build products
BUILD_DIR="${BASE_DIR}/build/darwin"
TEMP_DIR="${BUILD_DIR}/tmp"

# Clean previous output
rm -rf "${BUILD_DIR}"

mkdir -p ${TEMP_DIR}
if [[ $? != 0 ]]; then
  echo "FAILURE: mkdir -p ${TEMP_DIR}"
  exit 1
fi

# Ensure go is installed
which go 2>&1 > /dev/null
if [[ $? != 0 ]]; then
  echo "Go is not installed in the path, aborting"
  exit 1
fi

PRIVATE_PLUGINS_TAG=""
if [[ ${FORCE_PRIVATE_PLUGINS} == true ]]; then PRIVATE_PLUGINS_TAG="PRIVATE_PLUGINS"; fi

# Exporting these seems necessary for subcommands to pick them up.
export GOPATH=${TEMP_DIR}/go-darwin-build
export PATH=${GOPATH}/bin:${PATH}

# The GOPATH we're using is temporary, so make sure there isn't one from a previous run.
rm -rf ${GOPATH}

TUNNEL_CORE_SRC_DIR=${GOPATH}/src/github.com/Psiphon-Labs/psiphon-tunnel-core

mkdir -p ${GOPATH}
if [[ $? != 0 ]]; then
  echo "FAILURE: mkdir -p ${GOPATH}"
  exit 1
fi

# Symlink the current source directory into GOPATH, so that we're building the
# code in this local repo, rather than pulling from Github and building that.
mkdir -p ${GOPATH}/src/github.com/Psiphon-Labs
if [[ $? != 0 ]]; then
  echo "mkdir -p ${GOPATH}/src/github.com/Psiphon-Labs"
  exit 1
fi
ln -s "${BASE_DIR}/.." "${GOPATH}/src/github.com/Psiphon-Labs/psiphon-tunnel-core"
if [[ $? != 0 ]]; then
  echo "ln -s ../.. ${GOPATH}/src/github.com/Psiphon-Labs/psiphon-tunnel-core"
  exit 1
fi

# Check Go version
GO_VERSION=$(go version | sed -E -n 's/.*go([0-9]\.[0-9]+(\.[0-9]+)?).*/\1/p')
if [[ ${GO_VERSION} != ${GO_VERSION_REQUIRED} ]]; then
  echo "FAILURE: go version mismatch; require ${GO_VERSION_REQUIRED}; got ${GO_VERSION}"
  exit 1
fi

prepare_build () {

  # Ensure BUILD* variables reflect the tunnel-core repo
  cd ${TUNNEL_CORE_SRC_DIR}

  BUILDDATE=$(date +%Y-%m-%dT%H:%M:%S%z)
  BUILDREPO=$(git config --get remote.origin.url)
  BUILDREV=$(git rev-parse --short HEAD)
  GOVERSION=$(go version | perl -ne '/go version (.*?) / && print $1')

  # see DEPENDENCIES comment in MobileLibrary/Android/make.bash
  cd ${GOPATH}/src/github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary
  DEPENDENCIES=$(echo -n "{" && go list -tags "$1" -f '{{range $dep := .Deps}}{{printf "%s\n" $dep}}{{end}}' | xargs go list -f '{{if not .Standard}}{{.ImportPath}}{{end}}' | xargs -I pkg bash -c 'cd $GOPATH/src/pkg && echo -n "\"pkg\":\"$(git rev-parse --short HEAD)\","' | sed 's/,$/}/')

  LDFLAGS="\
  -X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.buildDate=$BUILDDATE \
  -X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.buildRepo=$BUILDREPO \
  -X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.buildRev=$BUILDREV \
  -X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.goVersion=$GOVERSION \
  -X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.dependencies=$DEPENDENCIES \
  "

  echo "Variables for ldflags:"
  echo " Build date: ${BUILDDATE}"
  echo " Build repo: ${BUILDREPO}"
  echo " Build revision: ${BUILDREV}"
  echo " Go version: ${GOVERSION}"
  echo " Dependencies: ${DEPENDENCIES}"
  echo ""

}


build_for_ios () {

  IOS_BUILD_TAGS="IOS ${PRIVATE_PLUGINS_TAG}"
  IOS_BUILD_DIR="${BUILD_DIR}/ios"
  rm -rf "${IOS_BUILD_DIR}"

  echo "...Getting project dependencies (via go get) for iOS."
  cd ${BASE_DIR}
  GOOS=darwin go get -d -v -tags "$IOS_BUILD_TAGS" ./...
  prepare_build "$IOS_BUILD_TAGS"
  if [ $? != 0 ]; then
      echo "....'go get' failed, exiting"
      exit $?
  fi

  curl https://raw.githubusercontent.com/golang/go/master/misc/ios/clangwrap.sh -o ${TEMP_DIR}/clangwrap.sh
  chmod 555 ${TEMP_DIR}/clangwrap.sh

  CC=${TEMP_DIR}/clangwrap.sh \
  CXX=${TEMP_DIR}/clangwrap.sh \
  CGO_LDFLAGS="-arch armv7 -isysroot $(xcrun --sdk iphoneos --show-sdk-path)" \
  CGO_CFLAGS=-isysroot$(xcrun --sdk iphoneos --show-sdk-path) \
  CGO_ENABLED=1 GOOS=darwin GOARCH=arm GOARM=7 go build -buildmode=c-archive -ldflags "$LDFLAGS" -tags "${IOS_BUILD_TAGS}" -o ${IOS_BUILD_DIR}/PsiphonTunnel-ios-arm.dylib PsiphonTunnel.go

  CC=${TEMP_DIR}/clangwrap.sh \
  CXX=${TEMP_DIR}/clangwrap.sh \
  CGO_LDFLAGS="-arch arm64 -isysroot $(xcrun --sdk iphoneos --show-sdk-path)" \
  CGO_CFLAGS=-isysroot$(xcrun --sdk iphoneos --show-sdk-path) \
  CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -buildmode=c-archive -ldflags "$LDFLAGS" -tags "${IOS_BUILD_TAGS}" -o ${IOS_BUILD_DIR}/PsiphonTunnel-ios-arm64.dylib PsiphonTunnel.go

}

build_for_macos () {

  MACOS_BUILD_TAGS="${PRIVATE_PLUGINS_TAG}"
  MACOS_BUILD_DIR="${BUILD_DIR}/macos"
  rm -rf "${MACOS_BUILD_DIR}"

  echo "...Getting project dependencies (via go get) for MacOS"
  cd ${BASE_DIR}
  GOOS=darwin go get -d -v -tags "$MACOS_BUILD_TAGS" ./...
  prepare_build "$MACOS_BUILD_TAGS"
  if [ $? != 0 ]; then
      echo "....'go get' failed, exiting"
      exit $?
  fi

  TARGET_ARCH=386
  CGO_ENABLED=1 GOOS=darwin GOARCH="${TARGET_ARCH}" go build -buildmode=c-archive -ldflags "$LDFLAGS" -tags "${MACOS_BUILD_TAGS}" -o "${MACOS_BUILD_DIR}/PsiphonTunnel-macos-${TARGET_ARCH}.dylib" PsiphonTunnel.go

  TARGET_ARCH=amd64
  CGO_ENABLED=1 GOOS=darwin GOARCH="${TARGET_ARCH}" go build -buildmode=c-archive -ldflags "$LDFLAGS" -tags "${MACOS_BUILD_TAGS}" -o "${MACOS_BUILD_DIR}/PsiphonTunnel-macos-${TARGET_ARCH}.dylib" PsiphonTunnel.go

}

cleanup () {
  # Remove temporary build artifacts
  rm -rf ${TEMP_DIR}
}


TARGET=$1
case $TARGET in
  macos)
    echo "..Building for MacOS"
    build_for_macos
    if [ $? != 0 ]; then
      exit $?
    fi

    ;;
  ios)
    echo "..Building for iOS"
    build_for_ios
    if [ $? != 0 ]; then
      exit $?
    fi

    ;;
  all)
    echo "..Building all"
    build_for_ios
    if [ $? != 0 ]; then
      exit $?
    fi

    build_for_macos
    if [ $? != 0 ]; then
      exit $?
    fi

    ;;
  *)
    echo "..No selection made, building all"
    build_for_ios
    if [ $? != 0 ]; then
      exit $?
    fi

    build_for_macos
    if [ $? != 0 ]; then
      exit $?
    fi

    ;;

esac

cleanup
echo "BUILD DONE"
