// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package version contains struct and convenience methods for representing, parsing and comparing Chrome versions.
package version

import (
	"regexp"
	"strconv"
	"strings"
)

// Version represents a Chrome version in the format of "(major).(minor).(build).(patch)".
// Unless stated otherwise, the Version methods expect to be called on valid versions.
type Version struct {
	components [4]int64
}

// IsValid checks if the version is set with valid numbers.
func (v *Version) IsValid() bool {
	return v.String() != "0.0.0.0"
}

// VersionRegexp is a regular expression to parse a Chrome version.
var VersionRegexp = regexp.MustCompile(`(\d+).(\d+).(\d+).(\d+)`)

// New creates a new instance of Version with version components.
func New(major, minor, build, patch int64) *Version {
	return &Version{
		components: [4]int64{major, minor, build, patch},
	}
}

// Delta represents the delta between two Chrome versions used to increment or decrement.
type Delta struct {
	components [4]int64
}

// NewDelta creates a new instance of Delta.
func NewDelta(major, minor, build, patch int64) *Delta {
	return &Delta{
		components: [4]int64{major, minor, build, patch},
	}
}

// Parse parses a version string of the form "(major).(minor).(build).(patch)". If parsing fails, it returns an invalid version.
func Parse(version string) Version {
	v := Version{}
	parts := VersionRegexp.FindStringSubmatch(version)
	if parts != nil {
		for id, part := range parts[1:] {
			number, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return Version{}
			}
			v.components[id] = number
		}
	}
	return v
}

// String returns a version string. e,g, "99.0.1.0"
func (v *Version) String() string {
	var version []string
	for _, component := range v.components {
		version = append(version, strconv.FormatInt(component, 10))
	}
	return strings.Join(version, ".")
}

// Major returns the major version component, also known as milestone.
func (v *Version) Major() int64 {
	return v.components[0]
}

// Minor returns the minor version component.
func (v *Version) Minor() int64 {
	return v.components[1]
}

// Build returns the build version component.
func (v *Version) Build() int64 {
	return v.components[2]
}

// Patch returns the patch version component.
func (v *Version) Patch() int64 {
	return v.components[3]
}

// Increment returns a copy of the receiver version, increased by the given components.
func (v *Version) Increment(d *Delta) Version {
	ret := *v
	ret.components[0] += d.components[0]
	ret.components[1] += d.components[1]
	ret.components[2] += d.components[2]
	ret.components[3] += d.components[3]
	return ret
}

// Decrement returns a copy of the receiver version, decreased by the given components.
// If any version component is less than zero, an invalid version ("0.0.0.0") will be returned for error handling on the callsite.
func (v *Version) Decrement(d *Delta) Version {
	ret := *v
	ret.components[0] -= d.components[0]
	ret.components[1] -= d.components[1]
	ret.components[2] -= d.components[2]
	ret.components[3] -= d.components[3]
	for _, component := range ret.components {
		if component < 0 {
			return Version{}
		}
	}
	return ret
}

// IsNewerThan compares two versions and returns true when lhs is newer than rhs.
func (v *Version) IsNewerThan(rhs Version) bool {
	for i, component := range v.components {
		o := rhs.components[i]
		if component != o {
			return component > o
		}
	}
	return false
}

// IsOlderThan compares two versions and returns true when lhs is older than rhs.
func (v *Version) IsOlderThan(rhs Version) bool {
	return rhs.IsNewerThan(*v)
}

// IsEqualTo returns true when the two versions are the same.
func (v *Version) IsEqualTo(rhs Version) bool {
	return v.components[0] == rhs.components[0] && v.components[1] == rhs.components[1] && v.components[2] == rhs.components[2] && v.components[3] == rhs.components[3]
}
