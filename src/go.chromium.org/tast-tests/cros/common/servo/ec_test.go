// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"testing"

	"go.chromium.org/tast/core/errors"
)

func TestErrorChecking(t *testing.T) {
	var err error
	err = &TabletModeCmdUnsupportedErr{E: errors.New("Some wrapped error")}

	if _, ok := err.(*TabletModeCmdUnsupportedErr); !ok {
		t.Error("Type cast didn't work")
	}
}
