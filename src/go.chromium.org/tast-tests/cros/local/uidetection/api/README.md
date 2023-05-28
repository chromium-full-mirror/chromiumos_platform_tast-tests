# uidetection API

This directory contains manually generated uidetection protobuf definitions for
go. This is to mitigate b/281456070 until there is a better solution.

To regenerate these files, run:
```
cd ~/
git clone git@github.com:googleapis/googleapis.git
cd googleapis

sudo apt install -y protobuf-compiler
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

protoc --go_out=. --go-grpc_out=. google/chromeos/uidetection/v1/ui_detection.proto
cp -r google.golang.org/genproto/googleapis/chromeos/uidetection/v1/* $CHROMEOS_SRC/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/uidetection/api
```
