// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package version

import (
	"testing"
)

func TestBasics(t *testing.T) {
	// New, Parse, IsEqualTo
	expected := New(100, 0, 1000, 1)
	parsed := Parse("100.0.1000.1")
	if !expected.IsEqualTo(parsed) {
		t.Errorf("%v IsEqualTo(%v), expected true", expected, parsed)
	}

	// IsValid
	for _, c := range []struct {
		v     Version
		valid bool
	}{
		{*New(1, 2, 3, 4), true},
		{Parse("1.2.3"), false},
		{Parse("a.b.c.d"), false},
	} {
		if c.v.IsValid() != c.valid {
			t.Errorf("%v IsValid, expected %v", c.v, c.valid)
		}
	}

	// IsOlderThan, IsNewerThan
	older := New(100, 0, 1000, 1)
	newer := New(100, 0, 9999, 1)
	if !older.IsOlderThan(*newer) {
		t.Errorf("%s IsOlderThan %s, expected true", older, newer)
	}
	if !newer.IsNewerThan(*older) {
		t.Errorf("%s IsNewerThan %s, expected true", newer, older)
	}

	// Increment, Decrement
	base := New(100, 0, 1000, 0)
	delta := NewDelta(0, 0, 100, 0)
	incremented := base.Increment(delta)
	want := New(100, 0, 1100, 0)
	if !incremented.IsEqualTo(*want) {
		t.Errorf("%s Increment(%v) == %v, want %s", base, delta, incremented, want)
	}
	decremented := incremented.Decrement(delta)
	want = base
	if !decremented.IsEqualTo(*want) {
		t.Errorf("%s Decrement(%v) == %v, want %s", base, delta, decremented, want)
	}
	// If it decrements less than zero, an invalid version 0.0.0.0 is returned.
	negative := base.Decrement(NewDelta(9999, 9999, 9999, 9999))
	if negative.IsValid() {
		t.Error("Decrement should not go less than zero")
	}
}
