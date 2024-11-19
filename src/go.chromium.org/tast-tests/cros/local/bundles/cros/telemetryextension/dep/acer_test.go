// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	gotesting "testing"
)

func TestAcerListIsSorted(t *gotesting.T) {
	for i := 1; i < len(acerModelList); i++ {
		if prev, cur := acerModelList[i-1], acerModelList[i]; prev >= cur {
			t.Errorf("Acer models are not in alphabetical order or contain duplicates: %q is followed by %q", prev, cur)
		}
	}
}
