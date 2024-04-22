// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"strings"
	"testing"
)

func TestVerifier_Succeed(t *testing.T) {
	resp := DumpHTTPResponse{
		URLs: []string{
			"api.google.com/foo/bar",
			"www.example.com",
		},
	}

	expectToSeen := []string{
		`api.google.com.*`,
		`www.example.com`,
	}

	expectedToNotSeen := []string{
		`www.expectedToNotSeen.com`,
	}

	bypassed := []string{
		`www.bypassed.com`,
	}

	v := NewNetworkVerifier(&resp)

	if err := v.Verify(expectToSeen, expectedToNotSeen, bypassed); err != nil {
		t.Fatal("failed to Verify a successful case", err)
	}
}

func TestVerifier_MissExpectToSee(t *testing.T) {
	resp := DumpHTTPResponse{
		URLs: []string{
			"api.google.com/foo/bar",
			"www.example.com",
		},
	}

	expectToSeen := []string{
		`api.google.com.*`,
		`www.example.com`,
		`www.example.org`,
	}

	expectedToNotSeen := []string{
		`www.expectedToNotSeen.com`,
	}

	bypassed := []string{
		`www.bypassed.com`,
	}

	v := NewNetworkVerifier(&resp)

	if err := v.Verify(expectToSeen, expectedToNotSeen, bypassed); err == nil {
		t.Fatal("test should fail but succeed")
	} else {
		if !strings.Contains(err.Error(), "www.example.org") {
			t.Fatal("test failed due to a wrong reason", err)
		}
	}
}

func TestVerifier_FindExpectNotToSee(t *testing.T) {
	resp := DumpHTTPResponse{
		URLs: []string{
			"api.google.com/foo/bar",
			"www.example.com",
		},
	}

	expectToSeen := []string{
		`api.google.com.*`,
	}

	expectedToNotSeen := []string{
		`www.example.com`,
	}

	bypassed := []string{
		`www.bypassed.com`,
	}

	v := NewNetworkVerifier(&resp)

	if err := v.Verify(expectToSeen, expectedToNotSeen, bypassed); err == nil {
		t.Fatal("test should fail but succeed")
	} else {
		if !strings.Contains(err.Error(), "www.example.com") {
			t.Fatal("test failed due to a wrong reason", err)
		}
	}
}

func TestVerifier_FindUnknownURL(t *testing.T) {
	resp := DumpHTTPResponse{
		URLs: []string{
			"api.google.com/foo/bar",
			"www.example.com",
			"www.unexpected.com",
		},
	}

	expectToSeen := []string{
		`api.google.com.*`,
		`www.example.com`,
	}

	expectedToNotSeen := []string{
		`www.expectedToNotSeen.com`,
	}

	bypassed := []string{
		`www.bypassed.com`,
	}

	v := NewNetworkVerifier(&resp)

	if err := v.Verify(expectToSeen, expectedToNotSeen, bypassed); err == nil {
		t.Fatal("test should fail but succeed")
	} else {
		if !strings.Contains(err.Error(), "www.unexpected.com") {
			t.Fatal("test failed due to a wrong reason", err)
		}
	}
}
