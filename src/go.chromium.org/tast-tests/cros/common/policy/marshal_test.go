// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"testing"
)

func TestOnePolicy(t *testing.T) {
	policy := &HomepageLocation{Val: "www.example.com", Stat: StatusSetRecommended}
	pJSON, err := Marshal(policy)
	if err != nil {
		t.Fatalf("Error marshalling policy into JSON: %s", err)
	}

	unmarshalledPolicy, err := Unmarshal(pJSON)
	if err != nil {
		t.Fatalf("Error unmarshalling policy from JSON %s: %s", pJSON, err)
	}

	typedPolicy, ok := unmarshalledPolicy.(*HomepageLocation)
	if !ok {
		t.Fatalf("Failed to recover correct policy type")
	}

	if typedPolicy.Val != policy.Val {
		t.Errorf("Incorrect value after unmarshalling: %s", typedPolicy.Val)
	}

	if typedPolicy.Stat != policy.Stat {
		t.Errorf("Incorrect stat after unmarshalling: %s", typedPolicy.Stat)
	}
}
func TestPolicyList(t *testing.T) {
	policies := []Policy{
		&AllowDinosaurEasterEgg{Val: true, Stat: StatusUnset},
		&HomepageLocation{Val: "asdf"},
	}

	pJSON, err := MarshalList(policies)
	if err != nil {
		t.Fatalf("Error marshalling policy list into JSON: %s", err)
	}

	unmarshalledList, err := UnmarshalList(pJSON)
	if err != nil {
		t.Fatalf("Error unmarshalling policy list from JSON %s: %s", pJSON, err)
	}

	// Check that length and names match. The serialization of individual fields is tested above.
	if len(unmarshalledList) != len(policies) {
		t.Fatalf("Incorrect number of policies after unmarshalling: %d", len(unmarshalledList))
	}

	for i, p := range policies {
		if unmarshalledList[i].Name() != p.Name() {
			t.Fatalf("Incorrect policy name at position #%d, expected: %s, got: %s", i, p.Name(), unmarshalledList[i].Name())
		}
	}
}
