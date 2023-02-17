// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nebraska contains helpers to run nebraska for dlc tests.
package nebraska

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/nebraska"
	"chromiumos/tast/testing"
)

// Nebraska struct hold Nebraska server runtime information for DLC tests.
type Nebraska struct {
	URL      string
	nebraska *nebraska.Nebraska
}

// Start starts the Nebraska server and returns the Nebraska struct on a
// successful bringup, otherwise an error is returned.
func Start(ctx context.Context) (*Nebraska, error) {
	instance, err := nebraska.Start(ctx, "/tmp/nebraska", []string{
		"--install-metadata", "/usr/local/dlc",
		"--install-payloads-address", "file:///usr/local/dlc",
	})
	if err != nil {
		return nil, err
	}

	return &Nebraska{
		URL:      fmt.Sprintf("http://127.0.0.1:%d/update?critical_update=True", instance.Port),
		nebraska: instance,
	}, nil
}

// Stop stops Nebraska, should pass in the Nebraska struct returned from Start.
func (n *Nebraska) Stop(ctx context.Context, s *testing.State, name string) error {
	if err := n.nebraska.Stop(ctx); err != nil {
		return err
	}

	if !s.HasError() {
		return nil
	}

	// Read nebraska log and dump it out.
	if b, err := ioutil.ReadFile("/tmp/nebraska.log"); err != nil {
		return errors.Wrap(err, "Nebraska log does not exist")
	} else if err := ioutil.WriteFile(filepath.Join(s.OutDir(), name+"-nebraska.log"), b, 0644); err != nil {
		return errors.Wrap(err, "failed to write nebraska log")
	}
	return nil
}
