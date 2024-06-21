// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crossdevice

// UnstableModels are devices on which cross device tests have not yet been stabilized.
var UnstableModels = []string{"dewatt", "rusty", "strongbad"}

// BackgroundScanningDisabledModels are models that do not support the background scanning feature.
var BackgroundScanningDisabledModels = []string{"babymega", "barla", "blooglet", "dewatt", "dumo", "fennel", "hana", "hayato", "kevin", "krane", "sentry", "soraka", "tomato", "treeya", "treeya360"}

// BGScanningStableSkipModels are models that should be skipped by stable BG scanning tests.
var BGScanningStableSkipModels = removeDuplicates(append(BackgroundScanningDisabledModels, UnstableModels...))

// BGScanningUnstableModels are models with background scanning enabled but need additional test stabilization.
var BGScanningUnstableModels = removeIntersection(UnstableModels, BackgroundScanningDisabledModels)

// removeDuplicates removes duplicate strings from the slice.
// Use this for combining model lists for HW dependencies.
func removeDuplicates(s []string) []string {
	m := make(map[string]bool)
	var dedup = []string{}
	for _, v := range s {
		if _, ok := m[v]; !ok {
			m[v] = true
			dedup = append(dedup, v)
		}
	}
	return dedup
}

// removeIntersection removes the intersection of s1 and s2 from s1.
// Use this for model HW dependencies where you want to run on models
// contained in s1 that are not in s2.
func removeIntersection(s1, s2 []string) []string {
	m := make(map[string]bool)
	for _, v := range s2 {
		m[v] = true
	}

	var s = []string{}
	for _, v := range s1 {
		if _, ok := m[v]; !ok {
			s = append(s, v)
		}
	}
	return s
}
