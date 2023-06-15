// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	gotesting "testing"
)

func TestLenovoListIsSorted(t *gotesting.T) {
	for i := 1; i < len(lenovoModelList); i++ {
		if prev, cur := lenovoModelList[i-1], lenovoModelList[i]; prev >= cur {
			t.Errorf("Lenovo models are not in alphabetical order or contain duplicates: %q is followed by %q", prev, cur)
		}
	}
}
