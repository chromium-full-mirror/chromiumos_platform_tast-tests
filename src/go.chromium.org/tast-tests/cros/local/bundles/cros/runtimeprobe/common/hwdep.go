// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common contains common things shared by runtime probe tests.
package common

import "go.chromium.org/tast/core/testing/hwdep"

// ReleasedDeviceDeps returns a hardware dependency that only runs a test on DUTs
// that has normal probe configs without private probe configs.
func ReleasedDeviceDeps(extraDeps ...hwdep.Condition) hwdep.Deps {
	deps := append(
		[]hwdep.Condition{hwdep.RuntimeProbeConfig(), hwdep.RuntimeProbeConfigPrivate(false)},
		extraDeps...,
	)
	return hwdep.D(deps...)
}

// UnreleasedDeviceDeps returns a hardware dependency that only runs a test on DUTs
// that has private probe configs.
func UnreleasedDeviceDeps(extraDeps ...hwdep.Condition) hwdep.Deps {
	deps := append(
		[]hwdep.Condition{hwdep.RuntimeProbeConfigPrivate(true)},
		extraDeps...,
	)
	return hwdep.D(deps...)
}
