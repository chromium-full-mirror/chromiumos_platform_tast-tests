// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"encoding/json"
	"strconv"

	"chromiumos/tast/errors"
)

// MarshalList marshals a list of policies into JSON that allows reconstructing
// the original policy types.
func MarshalList(policies []Policy) ([]byte, error) {
	var pJSONs []json.RawMessage
	for i, p := range policies {
		pJSON, err := Marshal(p)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal policy #"+strconv.Itoa(i))
		}
		pJSONs = append(pJSONs, pJSON)
	}

	return json.Marshal(pJSONs)
}

// UnmarshalList unmarshals a list of policies from JSON created by MarshalJSON
// func. It reconstructs the original policy types.
func UnmarshalList(data []byte) ([]Policy, error) {
	var pJSONs []json.RawMessage
	if err := json.Unmarshal(data, &pJSONs); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal policy list")
	}

	var policies []Policy
	for i, pJSON := range pJSONs {
		policy, err := Unmarshal(pJSON)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal policy #"+strconv.Itoa(i))
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

// Marshal marshals a policy into JSON that allows reconstructing the original
// policy type.
func Marshal(p Policy) ([]byte, error) {
	type PolicyProxy Policy
	b, err := json.Marshal(PolicyProxy(p))
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}

	m["$name"] = p.Name()
	return json.Marshal(m)
}

// Unmarshal unmarshals a policy from JSON created by Marshal func. It
// reconstructs the original policy type.
func Unmarshal(data []byte) (Policy, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}

	value, ok := m["$name"]
	if !ok {
		return nil, errors.New("Missing name field")
	}

	name, ok := value.(string)
	if !ok {
		return nil, errors.New("Name field is not a string")
	}

	policy, err := newByName(name)
	if err != nil {
		return nil, errors.Wrap(err, "unsupported policy name")
	}

	err = json.Unmarshal(data, &policy)
	if err != nil {
		return nil, err
	}

	return policy, nil
}
