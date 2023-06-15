// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	gotesting "testing"
)

func TestAsusHPHaveNoIntersection(t *gotesting.T) {
	for _, a := range asusModelList {
		for _, h := range hpModelList {
			if a == h {
				t.Errorf("Asus and HP have a common item: %q", a)
			}
		}
	}
}

func TestAsusLenovoHaveNoIntersection(t *gotesting.T) {
	for _, a := range asusModelList {
		for _, l := range lenovoModelList {
			if a == l {
				t.Errorf("Asus and Lenovo have a common item: %q", a)
			}
		}
	}
}

func TestHPLenovoHaveNoIntersection(t *gotesting.T) {
	for _, h := range hpModelList {
		for _, l := range lenovoModelList {
			if h == l {
				t.Errorf("HP and Lenovo have a common item: %q", h)
			}
		}
	}
}
