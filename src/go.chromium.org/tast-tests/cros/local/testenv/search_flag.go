// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testenv contains common constants, flags and utilities to be used by tests against external dependencies.
package testenv

import (
	"strings"

	"go.chromium.org/tast/core/testing"
)

// SearchFlagKey is the key for Search Flags to specify external dependency.
const SearchFlagKey = "external_dependency"

// ServiceDepName describes the available external dependency target name.
type ServiceDepName string

// Valid ServiceDepName.
// Note that the dependency target name is composed of:
//
//	"<service>.<environment>" in lower case, where
//
// <service> is any target service name,
// <environment> could be 'preprod' or any specific env name of their services.
const (
	// DMServer
	DMServerProd    ServiceDepName = "dmserver.prod"
	DMServerAlpha   ServiceDepName = "dmserver.alpha"
	DMServerStaging ServiceDepName = "dmserver.staging"

	// GFE
	GFEPreprod ServiceDepName = "gfe.preprod"
)

// SearchFlag generates a StringPair based on the given policy and ServiceDepName.
func SearchFlag(n ServiceDepName) *testing.StringPair {
	return &testing.StringPair{
		Key:   SearchFlagKey,
		Value: strings.ToLower(string(n)),
	}
}
