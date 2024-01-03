// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. device_info.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. one_time_metrics.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. local_info_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. power_status.proto

// Package powerpb defines helpers for transferring information in remote tests.
package powerpb

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/common/power/powerpb
