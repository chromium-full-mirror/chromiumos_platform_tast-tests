// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcheck provides common functions to check test definitions.
package testcheck

import (
	"strings"
	gotesting "testing"

	// We have to import graphics package so that fixture declaration chain can be established.
	_ "go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/testing"
	tastcheck "go.chromium.org/tast/core/testing/testcheck"
)

// allTests is trying to get all tests via tastcheck.Entities.
// This is not guaranteed to report complete result of the test informational registered in the GlobalRegistry.
func allTests() []*testing.TestInstance {
	var result []*testing.TestInstance
	for _, node := range tastcheck.Entities() {
		if node.Type != tastcheck.Test {
			continue
		}
		result = append(result, &testing.TestInstance{Name: node.Name})
	}
	return result
}

func getTests(t *gotesting.T, f tastcheck.TestFilter) []*testing.TestInstance {
	var tests []*testing.TestInstance
	for _, tst := range allTests() {
		if f(tst) {
			tests = append(tests, tst)
		}
	}
	if len(tests) == 0 {
		t.Fatalf("No tests matched")
	}
	return tests
}

// CheckFixtures checks that tests matched by f declare requiredFixtures either implicitly or explicitly in their fixture chain..
// requiredFixtures is a list of fixture names which needs to present in the test's Fixture chain.
// Each item in requiredFixture is one or '|'-connected multiple Fixture names.
// e.g.
// []string{"A", "B"} - fixture "A" and "B" both should present in the test's fixture chain.
// []string{"A|B"} - either fixture "A" or fixture "B" should present in the test's fixture chain.
//
// Note that this function is only traversing the local fixtures registered at the time this unit test is running.
// If any fixtures are not found in entities list, try importting the package which registered the fixture.
func CheckFixtures(t *gotesting.T, f tastcheck.TestFilter, requiredFixtures []string) {
	nodes := tastcheck.Entities()
	for _, tst := range getTests(t, f) {
		node, ok := nodes[tst.Name]
		if !ok {
			t.Errorf("%s, test not in entities list", tst.Name)
			continue
		}
		index := node.Parent
		fixtures := make(map[string]bool)
		for {
			if index == "" {
				break
			}
			node, ok := nodes[index]
			if !ok {
				t.Errorf("%s, failed to find in entities list. Is the fixture imported by this unit test?", index)
				break
			}
			fixtures[node.Name] = true
			index = node.Parent
		}
	CheckLoop:
		for _, fix := range requiredFixtures {
			for _, item := range strings.Split(fix, "|") {
				if _, ok := fixtures[item]; ok {
					continue CheckLoop
				}
			}
			t.Errorf("%s: missing fixture %q", tst.Name, fix)
		}
	}
}
