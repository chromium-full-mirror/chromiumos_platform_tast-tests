// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import "go.chromium.org/tast-tests/cros/local/arc"

// This file contains the available hook list.

// NewDumpARCOnFailureHook creates a hook which dumps network information inside
// on ARC on failures. s.AttachErrorHandlers() should be called to make this
// hook have effect. The passed in arc must be valid when the error happens. We
// can change the parameter to a closure to get arc if it might be changed in
// the test. The passed in arc can be nil, in which case the execution of this
// hook will be skipped. This might be helpful if the test only holds this
// object conditionally, so that it won't need to build the hook list
// conditionally.
func NewDumpARCOnFailureHook(arc *arc.ARC) hook {
	return &dumpARCOnFailureHook{a: arc}
}

// NewDumpHostOnFailureHook creates a hook which dumps network information in
// the host on failures. s.AttachErrorHandlers() should be called to make this
// hook have effect.
func NewDumpHostOnFailureHook() hook {
	return &dumpHostOnFailureHook{}
}

// NewSaveNetLogHook creates a hook which save the diff of net.log into the
// output folder of this test during the hook is running.
func NewSaveNetLogHook() hook {
	return &saveNetLogHook{}
}

// NewTcpdumpHook creates a hook which saves the packet dump into the output
// folder of this test during the hook is running.
func NewTcpdumpHook() hook {
	return &tcpdumpHook{}
}

// NewResetVirtualnetHook creates a hook which resets the states which may be
// changed by virtualnet or affect virtualnet environment, e.g., the
// EphemeralPriority on Services. It's recommended to have this hook if the test
// relies on virtualnet package. Note that the setup of this hook should happen
// before the setup of any virtualnet env, and the teardown of this hook should
// happen after the teardown of any virtualnet env, i.e., if the virtualnet
// setup is in the fixture, then having this hook in the test can cause
// unexpected behaviors.
func NewResetVirtualnetHook() hook {
	return &resetVirtualnetHook{}
}

// NewDisablePortalDetectionHook creates a hook which disables portal detection
// in shill in setup, and restores the portal detection config in teardown. This
// hook is helpful if the test doesn't care and wants to skip the network
// validation step so that the service state can go to online directly after
// connected.
func NewDisablePortalDetectionHook() hook {
	return &togglePortalDetectionHook{enable: false}
}

// NewEnablePortalDetectionHook creates a hook which enables portal detection in
// shill in setup, and restores the portal detection config in teardown. Note
// that portal detection is enabled by default, but a lot of tast tests change
// this value and may not do a proper cleanup. This hook is helpful to guarantee
// that the portal detection is enabled in the test.
func NewEnablePortalDetectionHook() hook {
	return &togglePortalDetectionHook{enable: true}
}
