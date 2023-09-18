// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. uwb_service.proto uwb_report.proto

// Package uwb is the supporting library that facilitates usage of UWB in CrOS testing.
package uwb

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/trunk/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/common/uwb
