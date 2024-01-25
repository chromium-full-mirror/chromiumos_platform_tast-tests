// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast/core/errors"
)

/*
This file implements miscellaneous and unsorted helpers.
*/

// ExpectAuthIntents checks whether two given sets of intents are equal, and
// in case they're not returns an error containing the formatted difference.
func ExpectAuthIntents(intents, expectedIntents []uda.AuthIntent) error {
	less := func(a, b uda.AuthIntent) bool { return a < b }
	diff := cmp.Diff(intents, expectedIntents, cmpopts.SortSlices(less))
	if diff == "" {
		return nil
	}
	return errors.New(diff)
}

// ExpectContainsAuthIntent checks whether the intents set contains the given value.
func ExpectContainsAuthIntent(intents []uda.AuthIntent, expectedIntent uda.AuthIntent) error {
	for _, intent := range intents {
		if intent == expectedIntent {
			return nil
		}
	}
	return errors.Errorf("expected to contain %v, got %v", expectedIntent, intents)
}

// ExpectAuthFactorTypes checks whether two given sets of types are equal, and
// in case they're not returns an error containing the formatted difference.
func ExpectAuthFactorTypes(types, expectedTypes []uda.AuthFactorType) error {
	less := func(a, b uda.AuthFactorType) bool { return a < b }
	diff := cmp.Diff(types, expectedTypes, cmpopts.SortSlices(less))
	if diff == "" {
		return nil
	}
	return errors.New(diff)
}

// ExpectContainsAuthFactorType checks whether the types set contains the given type.
func ExpectContainsAuthFactorType(types []uda.AuthFactorType, expectedType uda.AuthFactorType) error {
	for _, t := range types {
		if t == expectedType {
			return nil
		}
	}
	return errors.Errorf("expected to contain %v, got %v", expectedType, types)
}

// ExpectContainsAuthFactorTypes checks whether the first set of types contains
// the second set of types (i.e. the second set is a subset of the first).
func ExpectContainsAuthFactorTypes(types, containedTypes []uda.AuthFactorType) error {
	// Sort the two arrays.
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	sort.Slice(containedTypes, func(i, j int) bool { return containedTypes[i] < containedTypes[j] })
	// Go through the two arrays in parallel.
	var i, j int = 0, 0
	for i < len(types) && j < len(containedTypes) {
		if containedTypes[j] < types[i] {
			// We found an element in containedTypes but not in types.
			break
		} else if types[i] == containedTypes[j] {
			j++
		}
		i++
	}
	// If we haven't reached the end of containedTypes then this means that we
	// found an element in it that's not in types.
	if j < len(containedTypes) {
		return errors.Errorf("list %v does not contain %v", types, containedTypes[j])
	}
	// Otherwise, every element in containedTypes is in types.
	return nil
}

// ExpectAuthFactorsWithTypeAndLabel checks whether AuthFactorWithStatus proto
// contains expected AuthFactors, looking only at the types and labels of the
// factors. If they are not equal then this returns an error containing the
// formatted difference.
func ExpectAuthFactorsWithTypeAndLabel(factors, expectedFactors []*uda.AuthFactorWithStatus) error {
	eq := func(a, b *uda.AuthFactorWithStatus) bool {
		return a.AuthFactor.Type == b.AuthFactor.Type && a.AuthFactor.Label == b.AuthFactor.Label
	}
	less := func(a, b *uda.AuthFactorWithStatus) bool {
		return a.AuthFactor.Type < b.AuthFactor.Type || (a.AuthFactor.Type == b.AuthFactor.Type && a.AuthFactor.Label < b.AuthFactor.Label)
	}
	diff := cmp.Diff(factors, expectedFactors, cmp.Comparer(eq), cmpopts.SortSlices(less))
	if diff == "" {
		return nil
	}
	return errors.New(diff)
}

// ExpectCryptohomeErrorCode checks whether the specified error `err` has the
// exit error code equal to the specified `code`.
func ExpectCryptohomeErrorCode(err error, code uda.CryptohomeErrorCode) error {
	var exitErr *hwsec.CmdExitError
	if !errors.As(err, &exitErr) {
		return errors.Errorf("unexpected error: got %q; want *hwsec.CmdExitError", err)
	}
	if exitErr.ExitCode != int(code) {
		return errors.Errorf("unexpected exit code: got %d; want %d", exitErr.ExitCode, code)
	}
	return nil
}
