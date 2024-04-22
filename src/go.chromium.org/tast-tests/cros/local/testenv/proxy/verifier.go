// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"regexp"

	"go.chromium.org/tast/core/errors"
)

var (
	// ErrExpectedHostUnseen is the error to indicate unable to find host/url expected to see.
	ErrExpectedHostUnseen = errors.New("fail: not find host/url expected to see")
	// ErrUnexpectedHostSeen is the error to indicate able to find host/url expected to unseen.
	ErrUnexpectedHostSeen = errors.New("fail: find host/url expected to unseen")
	// ErrUnknownHost is the error to indicate able to find unknown host/url.
	ErrUnknownHost = errors.New("fail: find unknown host/url")
)

// NetworkVerifier is a struct to verify network traffic.
type NetworkVerifier struct {
	resp *DumpHTTPResponse
}

// NewNetworkVerifier is a method to create a new NetworkVerifier.
func NewNetworkVerifier(resp *DumpHTTPResponse) *NetworkVerifier {
	return &NetworkVerifier{
		resp: resp,
	}
}

// Verify is a method to verify URL.
// It has 3 parameters: expectedSeen, expectedNotSeen, bypassed.
// expectedSeen: []string. It supports regex. Everything in the list should be found.
// expectedNotSeen: []string. It supports regex. Everything in the list should not be found.
// bypassed: []string. It supports regex. Everything in the list may be found.
// Please note, all URLs should belong to one of three list.
// If an additional URL doesn't belong to any of list, the verification fails.
//
// Here are some examples:
// Case 1: it passes.
//
//	URLs: ["www.google.com", "www.youtube.com"]
//	expectedSeen: [`.*www.google.com.*`, `.*www.youtube.com.*`]
//	expectedNotSeen: []
//	bypassed: []
//
// Case 2: it fails because `.*www.example.com.*` in expectedSeen not found.
//
//	URLs: ["www.google.com", "www.youtube.com"]
//	expectedSeen: [`.*www.google.com.*`, `.*www.youtube.com.*`, `.*www.example.com.*`]
//	expectedNotSeen: []
//	bypassed: []
//
// Case 3: it fails because `.*www.youtube.com.*` in expectedNotSeen is found.
//
//	URLs: ["www.google.com", "www.youtube.com"]
//	expectedSeen: [`.*www.google.com.*`]
//	expectedNotSeen: [`.*www.youtube.com.*`]
//	bypassed: []
//
// Case 3: it fails because `.*www.youtube.com.*` is not in any list.
//
//	URLs: ["www.google.com", "www.youtube.com"]
//	expectedSeen: [`.*www.google.com.*`]
//	expectedNotSeen: []
//	bypassed: []
func (v *NetworkVerifier) Verify(expectedSeen, expectedNotSeen, bypassed []string) error {
	_, unmatched, _, unusedPatterns := match(expectedSeen, v.resp.URLs)

	if len(unusedPatterns) != 0 {
		return errors.Wrapf(ErrExpectedHostUnseen, "missing URL/hosts are %v", unusedPatterns)
	}

	matched, _, _, _ := match(expectedNotSeen, v.resp.URLs)

	if len(matched) != 0 {
		return errors.Wrapf(ErrUnexpectedHostSeen, "forbidden URL/hosts are %v", matched)
	}

	_, unmatched, _, _ = match(bypassed, unmatched)

	if len(unmatched) != 0 {
		return errors.Wrapf(ErrUnknownHost, "unknown URL/hosts are %v", unmatched)
	}

	return nil
}

func match(patterns, targets []string) (matchedTargets, unmatchedTargets, usedPatterns, unusedPatterns []string) {
	// usedPatternsMap is to track whether a pattern is used.
	usedPatternsMap := make(map[string]bool)
	for _, p := range patterns {
		usedPatternsMap[p] = false
	}

	// matchedTargetsMap is to track whether a target is found
	matchedTargetsMap := make(map[string]bool)
	for _, t := range targets {
		matchedTargetsMap[t] = false
	}

	for _, t := range targets {
		for _, p := range patterns {
			compiledPattern := regexp.MustCompile(p)
			if compiledPattern.MatchString(t) {
				matchedTargetsMap[t] = true
				usedPatternsMap[p] = true
				break
			}
		}
	}

	for k, v := range matchedTargetsMap {
		if v {
			matchedTargets = append(matchedTargets, k)
		} else {
			unmatchedTargets = append(unmatchedTargets, k)
		}
	}

	for k, v := range usedPatternsMap {
		if v {
			usedPatterns = append(usedPatterns, k)
		} else {
			unusedPatterns = append(unusedPatterns, k)
		}
	}

	return
}
