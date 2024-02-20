// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common contains common things shared by runtime probe tests.
package common

import "go.chromium.org/tast/core/testing/hwdep"

// ReleasedDeviceDeps is a hardware dependency that only runs a test on DUTs
// that has normal probe configs without private probe configs.
var ReleasedDeviceDeps = hwdep.D(hwdep.RuntimeProbeConfig(), hwdep.RuntimeProbeConfigPrivate(false))

// UnreleasedDeviceDeps is a hardware dependency that only runs a test on DUTs
// that has private probe configs.
var UnreleasedDeviceDeps = hwdep.D(hwdep.RuntimeProbeConfigPrivate(true))
