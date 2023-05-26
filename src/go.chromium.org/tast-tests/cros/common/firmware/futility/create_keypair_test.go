// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"
	"testing"
)

func TestCreateKeyPairNoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)
	opts := NewCreateKeyPairOptions("")

	_, err := i.CreateKeyPair(context.Background(), opts)
	if want := "input file cannot be empty"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %v, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func TestCreateKeyPairOpts(t *testing.T) {
	for _, p := range []struct {
		args   []string
		setter func(o *createKeyPairOptions) *createKeyPairOptions
	}{
		{[]string{"--version", "2"}, func(o *createKeyPairOptions) *createKeyPairOptions { return o.WithVersion(2) }},
		{[]string{"--hash_alg", fmt.Sprintf("%d", HashAlgorithmSHA512)}, func(o *createKeyPairOptions) *createKeyPairOptions { return o.WithHashAlgorithm(HashAlgorithmSHA512) }},
		{[]string{"--id", "0123456789abcdef"}, func(o *createKeyPairOptions) *createKeyPairOptions { return o.WithID("0123456789abcdef") }},
		{[]string{"--desc", "Description string"}, func(o *createKeyPairOptions) *createKeyPairOptions { return o.WithDescription("Description string") }},
	} {
		i := newTestInstance(nil, nil, nil)
		opts := p.setter(NewCreateKeyPairOptions("input.pem").WithBaseName("output/path"))
		positionalArgs := []string{testFutilityPath, "create", "input.pem", "output/path"}

		if _, err := i.CreateKeyPair(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		ti := i.commandRunner.(*testCommandRunner)
		if err := ti.assertCalledWith(positionalArgs, [][]string{p.args}, nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}
