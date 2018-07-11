#!/usr/bin/env bash

# -x echos commands. 
# -e exits if a command returns an error.
set -x -e

# This script takes one optional argument: 'private', if private plugins should
# be used. It should be omitted if private plugins are not desired.
if [[ $1 == "private" ]]; then
  FORCE_PRIVATE_PLUGINS=true
  echo "TRUE"
else
  FORCE_PRIVATE_PLUGINS=false
  echo "FALSE"
fi

# -u exits if an unintialized variable is used.
set -u

# Modify this value as we use newer Go versions.
GO_VERSION_REQUIRED="1.9.6"

BASE_DIR=$(cd "$(dirname "$0")" ; pwd -P)
cd ${BASE_DIR}

# The location of the final build products
BUILD_DIR="${BASE_DIR}/build/iOS"
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
BUILD_TAGS="IOS ${PRIVATE_PLUGINS_TAG}"

# Exporting these seems necessary for subcommands to pick them up.
export GOPATH=${PWD}/build/go-darwin-build
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

# Get dependencies
echo "...Getting project dependencies (via go get) for iOS."
GOOS=darwin go get -d -v -tags "$BUILD_TAGS" ./...

#
# Build
#

# Ensure BUILD* variables reflect the tunnel-core repo
cd ${TUNNEL_CORE_SRC_DIR}

BUILDDATE=$(date +%Y-%m-%dT%H:%M:%S%z)
BUILDREPO=$(git config --get remote.origin.url)
BUILDREV=$(git rev-parse --short HEAD)
GOVERSION=$(go version | perl -ne '/go version (.*?) / && print $1')

# see DEPENDENCIES comment in MobileLibrary/Android/make.bash
cd ${GOPATH}/src/github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary
DEPENDENCIES=$(echo -n "{" && go list -tags "${BUILD_TAGS}" -f '{{range $dep := .Deps}}{{printf "%s\n" $dep}}{{end}}' | xargs go list -f '{{if not .Standard}}{{.ImportPath}}{{end}}' | xargs -I pkg bash -c 'cd $GOPATH/src/pkg && echo -n "\"pkg\":\"$(git rev-parse --short HEAD)\","' | sed 's/,$/}/')

LDFLAGS="\
-X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.buildDate=${BUILDDATE} \
-X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.buildRepo=${BUILDREPO} \
-X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.buildRev=${BUILDREV} \
-X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.goVersion=${GOVERSION} \
-X github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common.dependencies=${DEPENDENCIES} \
"

echo ""
echo "Variables for ldflags:"
echo " Build date: ${BUILDDATE}"
echo " Build repo: ${BUILDREPO}"
echo " Build revision: ${BUILDREV}"
echo " Go version: ${GOVERSION}"
echo ""

curl https://raw.githubusercontent.com/golang/go/master/misc/ios/clangwrap.sh -o ${TEMP_DIR}/clangwrap.sh
chmod 555 ${TEMP_DIR}/clangwrap.sh

CC=${TEMP_DIR}/clangwrap.sh \
CXX=${TEMP_DIR}/clangwrap.sh \
CGO_LDFLAGS="-arch armv7 -isysroot $(xcrun --sdk iphoneos --show-sdk-path)" \
CGO_CFLAGS=-isysroot$(xcrun --sdk iphoneos --show-sdk-path) \
CGO_ENABLED=1 GOOS=darwin GOARCH=arm GOARM=7 go build -buildmode=c-archive -ldflags "$LDFLAGS" -tags "${BUILD_TAGS}" -o ${BUILD_DIR}/PsiphonTunnel-darwin-arm.dylib PsiphonTunnel.go

CC=${TEMP_DIR}/clangwrap.sh \
CXX=${TEMP_DIR}/clangwrap.sh \
CGO_LDFLAGS="-arch arm64 -isysroot $(xcrun --sdk iphoneos --show-sdk-path)" \
CGO_CFLAGS=-isysroot$(xcrun --sdk iphoneos --show-sdk-path) \
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -buildmode=c-archive -ldflags "$LDFLAGS" -tags "${BUILD_TAGS}" -o ${BUILD_DIR}/PsiphonTunnel-darwin-arm64.dylib PsiphonTunnel.go

# Remove temporary build artifacts
 rm -rf ${TEMP_DIR}

echo "BUILD DONE"

