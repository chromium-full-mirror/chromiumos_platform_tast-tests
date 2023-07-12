// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. utils_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. bios_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. fp_updater_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. serial_port_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. kernel_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. tpm_service.proto

package firmware

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/firmware
