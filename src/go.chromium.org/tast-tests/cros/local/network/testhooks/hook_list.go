// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import "go.chromium.org/tast-tests/cros/local/arc"

// This file contains the available hook list.

// NewDumpARCOnFailureHook creates a hook which dumps network information
// inside on ARC on failures. s.AttachErrorHandlers() should be called to make
// this hook have effect. The passed in arc must be valid when the error
// happens. We can change the parameter to a closure to get arc if it might be
// changed in the test.
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
