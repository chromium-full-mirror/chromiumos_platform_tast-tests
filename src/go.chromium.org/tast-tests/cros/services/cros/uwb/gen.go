// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. --proto_path=../../../common/uwb --proto_path=./ uwb_service.proto uwb_report.proto uwb_remote_service.proto

// Package uwb contains the protobufs and its corresponding generated Go code that allows remote testing of UWB in ChromeOS.
package uwb

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/trunk/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/uwb
