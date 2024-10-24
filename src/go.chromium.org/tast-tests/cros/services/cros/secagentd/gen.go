// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd provides the FileEventService.
package secagentd

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/secagentd
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. fileevent_service.proto
