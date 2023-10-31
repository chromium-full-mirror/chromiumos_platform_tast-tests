// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I ~/chromiumos/src/platform2/metrics/structured/proto/ --go_out=plugins=grpc:../../../../../.. structured_data.proto

package proto

// protos are in ~/chromiumos/src/platform2/metrics/structured/proto/
// Only structured_data.proto is needed for current tests.
// Run command above in this location to generate the corresponding pb.go file.
// Requires `option go_package = "go.chromium.org/tast-tests/cros/local/metrics/proto";`
// in structured_data.proto.

// TODO(b/307536722): Manual generation of the proto is not ideal, even if it is
// not expected to change a lot. Generate and include the proto automatically as
// part of the BUILD.
