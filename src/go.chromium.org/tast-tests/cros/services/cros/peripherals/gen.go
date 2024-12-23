// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. periph_service.proto

// Package peripherals provides all peripherals related types compiled from protobuf.
package peripherals

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/peripherals
