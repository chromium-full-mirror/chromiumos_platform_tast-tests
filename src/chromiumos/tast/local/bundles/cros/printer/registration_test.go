// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"testing"

	"chromiumos/tast/testing/testcheck"
)

func TestSoftwareDeps(t *testing.T) {
	testcheck.SoftwareDeps(t, testcheck.Glob(t, "printer.*"), []string{"cups"})
	testcheck.SoftwareDeps(t, testcheck.Glob(t, "printer.Add.*"), []string{"ghostscript"})
	testcheck.SoftwareDeps(t, testcheck.Glob(t, "printer.Gs*"), []string{"ghostscript"})
	testcheck.SoftwareDeps(t, testcheck.Glob(t, "printer.Proxy*"), []string{"chrome", "ghostscript", "plugin_vm"})
}
