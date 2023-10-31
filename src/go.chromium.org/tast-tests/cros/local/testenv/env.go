// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"encoding/json"

	"go.chromium.org/tast/core/errors"
)

// Hostname represents a hostname of external endpoint. eg, "foobarbaz.com"
type Hostname string

// hostRegistry contains all identified hostnames of managed services
// in their prod or preprod environments.
// It will be initialized with a predefined .yaml data file.
type hostRegistry struct {
	hostMap map[string]Hostname
}

func newHostRegistry(ctx context.Context, hostsJSON string) (*hostRegistry, error) {
	if hostsJSON == "" {
		return nil, errors.New("hostsJSON can't be empty")
	}
	hr := &hostRegistry{}
	if err := json.Unmarshal([]byte(hostsJSON), &hr.hostMap); err != nil {
		return nil, errors.Wrap(err, "failed to decode hosts in json")
	}
	return hr, nil
}

// hostRouteRules specifies the rules used for overriding hostnames to set up
// the wanted test environment.
// This allows tests to point to IP addresses of given target hosts.
// It will be initialized with a predefined .yaml data file.
type hostRouteRules struct {
	rules map[string][]fromTo
}

// fromTo is a pair of (from, to) used for host override.
type fromTo struct {
	From Hostname `json:"from"`
	To   Hostname `json:"to"`
}

func newHostRouteRules(ctx context.Context, rulesJSON string) (*hostRouteRules, error) {
	if rulesJSON == "" {
		return nil, errors.New("rulesJSON can't be empty")
	}
	hrr := &hostRouteRules{rules: make(map[string][]fromTo)}
	if err := json.Unmarshal([]byte(rulesJSON), &hrr.rules); err != nil {
		return nil, errors.Wrap(err, "failed to decode rules in json")
	}
	return hrr, nil
}

// Env provides the user-facing interfaces that are to be commonly used to turn
// up or down the test environments.
// It encapsutes handling the host registry and the rules to hide details and
// make the common use case as simple as possible.
type Env interface {
	SetUp(context.Context) error
	TearDown(context.Context) error
}

type baseEnv struct {
	Env
	name     string
	registry *hostRegistry
	rules    *hostRouteRules
}

// SetUp is an abstract interface that should be implemented in a concrete Env struct.
func (baseEnv) SetUp(context.Context) error {
	return errors.New("SetUp should be implemented in concrete Env")
}

// TearDown is an abstract interface that should be implemented in a concrete Env struct.
func (baseEnv) TearDown(ctx context.Context) error {
	return errors.New("TearDown should be implemented in concrete Env")
}

func newBaseEnv(ctx context.Context, name, hostsJSON, rulesJSON string) (*baseEnv, error) {
	registry, err := newHostRegistry(ctx, hostsJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init the host registry")
	}
	rules, err := newHostRouteRules(ctx, rulesJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init the host route rules")
	}
	return &baseEnv{
		name:     name,
		registry: registry,
		rules:    rules,
	}, nil
}
