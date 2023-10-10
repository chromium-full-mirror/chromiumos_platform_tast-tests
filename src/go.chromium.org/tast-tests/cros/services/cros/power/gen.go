// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . -I ../../../common/perf/perfpb --go_out=plugins=grpc:../../../../../.. metrics_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. battery_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. device_setup_service.proto
//go:generate protoc -I . -I ../../../common/power/powerpb -I ../../../common/perf/perfpb --go_out=plugins=grpc:../../../../../.. recorder_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. usb_service.proto

// Package power provides the BatteryService.
package power

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/power
