// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vm provides the VMService
package vm

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/vm
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. vm_service.proto
