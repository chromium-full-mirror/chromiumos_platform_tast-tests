// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
)

// pinweaverManagerBinary is used to interact with the pinweaver_manager backend
// in libhwsec over 'pinweaver_manager_client' executable. For more details of
// the arguments of the functions in this file, please check //src/platform2/libhwsec/client/pinweaver_manager.cc.
type pinweaverManagerBinary struct {
	runner CmdRunner
}

// newPinWeaverManagerBinary is a factory function to create a
// deviceManagementBinary instance.
func newPinWeaverManagerBinary(r CmdRunner) *pinweaverManagerBinary {
	return &pinweaverManagerBinary{r}
}

// call is a simple utility that helps to call device_management_client.
func (c *pinweaverManagerBinary) call(ctx context.Context, args ...string) (string, error) {
	out, err := c.runner.Run(ctx, "pinweaver_manager_client", args...)
	return string(out), err
}

// syncHashTree calls "pinweaver_manager_client sync".
func (c *pinweaverManagerBinary) syncHashTree(ctx context.Context) (string, error) {
	return c.call(ctx, "sync")
}

// insertCredential calls "pinweaver_manager_client insert <le_secret> <he_secret> <reset_secret>".
func (c *pinweaverManagerBinary) insertCredential(ctx context.Context, leSecret, heSecret, resetSecret string) (string, error) {
	return c.call(ctx, "insert", leSecret, heSecret, resetSecret)
}

// checkCredential calls "pinweaver_manager_client auth <label> <le_secret>".
func (c *pinweaverManagerBinary) checkCredential(ctx context.Context, label, leSecret string) (string, error) {
	return c.call(ctx, "auth", label, leSecret)
}

// resetCredential calls "pinweaver_manager_client reset <label> <reset_secret>".
func (c *pinweaverManagerBinary) resetCredential(ctx context.Context, label, resetSecret string) (string, error) {
	return c.call(ctx, "reset", label, resetSecret)
}

// removeCredential calls "pinweaver_manager_client remove <label>".
func (c *pinweaverManagerBinary) removeCredential(ctx context.Context, label string) (string, error) {
	return c.call(ctx, "remove", label)
}
