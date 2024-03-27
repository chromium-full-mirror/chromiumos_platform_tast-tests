// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DefaultLeSecret default low entropy secret of a PinWeaverLabel
	DefaultLeSecret = "le_secret"
	// DefaultHeSecret default high entropy secret of a PinWeaverLabel
	DefaultHeSecret = "he_secret"
	// DefaultResetSecret default reset secret of a PinWeaverLabel
	DefaultResetSecret = "reset_secret"
	// WrongSecret just another secret with value different from the above three ones
	WrongSecret = "wrong_secret"
)

// PinWeaverManagerClient wraps and the functions of pinweaverManagerBinary and parses the outputs to
// structured data.
type PinWeaverManagerClient struct {
	binary *pinweaverManagerBinary
}

// NewPinWeaverManagerClient creates a new PinWeaverManagerClient.
func NewPinWeaverManagerClient(r CmdRunner) *PinWeaverManagerClient {
	return &PinWeaverManagerClient{
		binary: newPinWeaverManagerBinary(r),
	}
}

// SyncHashTree retrieves the credential values of a leaf, and returns the tuple (value, error), whereby value is the label of the leaf, and error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *PinWeaverManagerClient) SyncHashTree(ctx context.Context) error {
	out, err := u.binary.syncHashTree(ctx)
	testing.ContextLog(ctx, strings.Split(strings.TrimSpace(out), "\n"))
	if err != nil {
		return errors.Wrapf(err, "failed to sync pinweaver hash tree with the following output %q", out)
	}
	return nil
}

// CreateDefaultCredential calls insertCredential with default secrets set
func (u *PinWeaverManagerClient) CreateDefaultCredential(ctx context.Context) (int, error) {
	return u.insertCredential(ctx, DefaultLeSecret, DefaultHeSecret, DefaultResetSecret)
}

// CreateLockedOutCredential calls insertCredential with default secrets set, then locked out the credential by performing 5 checkCredential calls with incorrect le_secret
func (u *PinWeaverManagerClient) CreateLockedOutCredential(ctx context.Context) (int, error) {
	label, err := u.insertCredential(ctx, DefaultLeSecret, DefaultHeSecret, DefaultResetSecret)
	if err != nil {
		return label, err
	}

	// Lock out the label by performing 5 incorrect CheckCredential attempts.
	for i := 1; i <= 5; i++ {
		u.CheckCredential(ctx, label, WrongSecret)
	}

	// Verify that the label is locked out.
	if _, _, err = u.CheckCredential(ctx, label, DefaultLeSecret); err == nil {
		return -1, errors.Wrap(err, "failed to locked out pinweaver credential")
	}
	return label, nil
}

// insertCredential retrieves the credential values of a leaf, and returns the tuple (value, error), whereby value is the label of the leaf, and error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *PinWeaverManagerClient) insertCredential(ctx context.Context, leSecret, heSecret, resetSecret string) (int, error) {
	out, err := u.binary.insertCredential(ctx, leSecret, heSecret, resetSecret)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to insert pinweaver credential with the following output %q", out)
	}
	return u.parseLabel(out)
}

// CheckCredential retrieves the credential values of a leaf, and returns the tuple (value, error), whereby value is the label of the leaf, and error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *PinWeaverManagerClient) CheckCredential(ctx context.Context, label int, leSecret string) (string, string, error) {
	out, err := u.binary.checkCredential(ctx, strconv.Itoa(label), leSecret)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to check pinweaver credential with the following output %q", out)
	}
	heSecret, err := u.parseString(out, "he_secret")
	resetSecret, err2 := u.parseString(out, "reset_secret")
	if err != nil || err2 != nil {
		return "", "", errors.Wrapf(err, "failure in pinWeaverManagerParseStringHelper, output %q", out)
	}
	return heSecret, resetSecret, nil
}

// ResetCredential retrieves the credential values of a leaf, and returns the tuple (value, error), whereby value is the label of the leaf, and error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *PinWeaverManagerClient) ResetCredential(ctx context.Context, label int, resetSecret string) error {
	out, err := u.binary.resetCredential(ctx, strconv.Itoa(label), resetSecret)
	if err != nil {
		return errors.Wrapf(err, "failed to reset pinweaver credential with the following output %q", out)
	}
	return nil
}

// RemoveCredential retrieves the credential values of a leaf, and returns the tuple (value, error), whereby value is the label of the leaf, and error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *PinWeaverManagerClient) RemoveCredential(ctx context.Context, label int) error {
	out, err := u.binary.removeCredential(ctx, strconv.Itoa(label))
	if err != nil {
		return errors.Wrapf(err, "failed to remove pinweaver credential with the following output %q", out)
	}
	return nil
}

// parseString a helper function that helps to parse the output of PinweaverManagerClient.
func (u *PinWeaverManagerClient) parseString(out, key string) (string, error) {
	var value string
	for _, s := range strings.Fields(out) {
		if n, err := fmt.Sscanf(s, key+"=%s", &value); err == nil && n == 1 {
			// We've found the output that we need.
			return value, nil
		}
	}
	return "", errors.Errorf("failed to parse PinWeaverManager output, got %s", out)
}

// parseLabel a helper function that helps to parse the label(uint) from the output of PinweaverManagerClient.
func (u *PinWeaverManagerClient) parseLabel(out string) (int, error) {
	var value int
	key := "label"
	for _, s := range strings.Fields(out) {
		if n, err := fmt.Sscanf(s, key+"=%d", &value); err == nil && n == 1 {
			// We've found the output that we need.
			return value, nil
		}
	}
	return -1, errors.Errorf("failed to parse PinWeaverManager output, got %s", out)
}
