// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"encoding/json"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// hostRegistry contains a map of labels to hostnames.
// Label must be suffixed with the valid env name (eg, -preprod, -prod)
// It will be initialized with a predefined .yaml data file.
type hostRegistry struct {
	hostMap map[string]string
}

func newHostRegistry(ctx context.Context, hostsJSON string) (*hostRegistry, error) {
	if hostsJSON == "" {
		return nil, errors.New("hostsJSON can't be empty")
	}
	// Lowercase the JSON string for easy comparison
	hostsJSON = strings.ToLower(hostsJSON)

	hr := &hostRegistry{}
	if err := json.Unmarshal([]byte(hostsJSON), &hr.hostMap); err != nil {
		return nil, errors.Wrap(err, "failed to decode hosts in json")
	}

	// Check the validity.
	for label, hostname := range hr.hostMap {
		if !isValidLabel(label) {
			return nil, errors.Errorf("invalid label: %v", label)
		}
		if !isValidHostname(hostname) {
			return nil, errors.Errorf("invalid hostname associated with label: %v", label)
		}
	}
	return hr, nil
}

// routeRules contains a map of rules that is used to route traffic from one host to another, usually in preprod.
// Label is used to specify route info instead of hostname directly.
// It will be initialized with a predefined .yaml data file.
// eg, {"default": [{from: "foobarbaz-prod", to: "foobarbaz-preprod"}, ...], ...}
type routeRules struct {
	rules map[string][]struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
}

func newRouteRules(ctx context.Context, rulesJSON string) (*routeRules, error) {
	if rulesJSON == "" {
		return nil, errors.New("rulesJSON can't be empty")
	}
	// Lowercase the JSON string for easy comparison
	rulesJSON = strings.ToLower(rulesJSON)

	hrr := &routeRules{}
	if err := json.Unmarshal([]byte(rulesJSON), &hrr.rules); err != nil {
		return nil, errors.Wrap(err, "failed to decode rules in json")
	}

	// Check the validity.
	for _ /*rulename*/, r := range hrr.rules {
		for _, fromTo := range r {
			if !isValidLabel(fromTo.From) || !isValidLabel(fromTo.To) {
				return nil, errors.Errorf("invalid label: %v", fromTo)
			}
		}
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
	route    *routeRules
}

func newBaseEnv(ctx context.Context, name, hostsJSON, rulesJSON string) (*baseEnv, error) {
	registry, err := newHostRegistry(ctx, hostsJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init the host registry")
	}
	route, err := newRouteRules(ctx, rulesJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init the host route rules")
	}
	return &baseEnv{
		name:     name,
		registry: registry,
		route:    route,
	}, nil
}

// SetUp is an abstract interface that should be implemented in a concrete Env struct.
func (baseEnv) SetUp(context.Context) error {
	return errors.New("SetUp should be implemented in concrete Env")
}

// TearDown is an abstract interface that should be implemented in a concrete Env struct.
func (baseEnv) TearDown(ctx context.Context) error {
	return errors.New("TearDown should be implemented in concrete Env")
}

// hostInfo contains info about a host to be passed to the env setup.
type hostInfo struct {
	hostname string // host name, don't expose in the log. eg, "foo.bar.baz"
	label    string // label, should have the env name as a suffix. eg, "foobarbaz-preprod"
}

// fromTo is a pair of (from, to) of host info
type fromTo struct {
	from hostInfo
	to   hostInfo
}

// destRules returns a list of the rules pointing to hosts in the given environment.
func (e baseEnv) destRules(ctx context.Context) ([]fromTo, error) {
	// Consider adding a new rule name other than "default" when needed for certain scenarios. The var .yaml file should be updated accordingly.
	const defaultRuleName = "default"
	labels, ok := e.route.rules[defaultRuleName]
	if !ok {
		return nil, errors.Errorf("no route rules found for %v env with name: %v", e.name, defaultRuleName)
	}

	var dests []fromTo
	for _, label := range labels {
		// Find the rule that has a suffix that matches the current environment.
		if !strings.HasSuffix(string(label.To), "-"+e.name) {
			continue
		}
		dest := fromTo{
			from: hostInfo{hostname: e.registry.hostMap[label.From], label: label.From},
			to:   hostInfo{hostname: e.registry.hostMap[label.To], label: label.To},
		}
		dests = append(dests, dest)
	}
	if len(dests) == 0 {
		return nil, errors.Errorf("no route rules point to %v env as destination", e.name)
	}
	return dests, nil
}
