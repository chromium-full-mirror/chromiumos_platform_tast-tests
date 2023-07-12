// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. file_system.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. faillog_service.proto

// Package baserpc defines gRPC services common to tests.
package baserpc

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/baserpc
