// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"fmt"
	"testing"
)

func TestFwInfoTypeKeyIDDefaultValueString(t *testing.T) {
	if got := fmt.Sprintf("%v", fwInfoTypeKeyID); got != "fwInfoTypeKeyID" {
		t.Fatalf("Printing fwInfoTypeKeyID yielded %q, want %q", got, "fwInfoTypeKeyID")
	}
}
