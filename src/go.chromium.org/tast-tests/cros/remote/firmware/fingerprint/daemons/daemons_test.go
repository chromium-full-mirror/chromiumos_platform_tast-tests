// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package daemons

import (
	"testing"
)

func TestUpstartJobStringWithoutArgs(t *testing.T) {
	j := UpstartJob{
		Name: "serv",
	}
	expectedString := "serv"
	actualString := j.String()
	if actualString != expectedString {
		t.Errorf("String() returned %q; want %q", actualString, expectedString)
	}
}

func TestUpstartJobStringWithArgs(t *testing.T) {
	j := UpstartJob{
		Name: "serv",
		Args: map[string]string{
			"LOG":  "/var/log/serv.log",
			"PORT": "9999",
		},
	}
	expectedString := "serv(LOG=/var/log/serv.log PORT=9999)"
	actualString := j.String()
	if actualString != expectedString {
		t.Errorf("String() returned %q; want %q", actualString, expectedString)
	}
}
