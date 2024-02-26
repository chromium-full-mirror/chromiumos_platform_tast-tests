// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"context"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VmmmsInitVerifier verifies that the Virtual Machine Memory Management Service
// initialized properly.
type VmmmsInitVerifier struct {
	reader *syslog.Reader
}

// NewVmmmsInitVerifier starts scanning logs and returns a VmmmsInitVerifier
// which makes sure VMMMS initialized properly. Should be called before a new
// Chrome instance is started.
func NewVmmmsInitVerifier(ctx context.Context) (*VmmmsInitVerifier, error) {
	reader, err := syslog.NewReader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open syslog to verify VMMMS initialization")
	}
	return &VmmmsInitVerifier{reader}, nil
}

// Close must be called to free resources.
func (iv *VmmmsInitVerifier) Close() {
	iv.reader.Close()
}

// vmmmsServerRE matches VMMMS log lines for client connections, e.g.:
// Server accepted new connection. CID: 32 Type: CONNECTION_TYPE_KILLS
var vmmmsServerRE = regexp.MustCompile(`^Server accepted new connection\. CID: (\d+) Type: (\w+)$`)

// Verify scans syslog from the VmmmsInitVerifier creation time looking for
// lines showing Resource Manager and ARC connecting to VMMMS. Should be called
// after arc.New* to make sure VMMMS has had enough time to initialize.
func (iv *VmmmsInitVerifier) Verify(ctx context.Context) error {
	version, _, err := sysutil.KernelVersionAndArch()
	if err != nil {
		return errors.Wrap(err, "failed to get kernel version to check that VMMMS is supported")
	}
	// VMMMMS is only supported on ChromeOS Kernels 5.4 and later.
	if !version.IsOrLater(5, 4) {
		testing.ContextLogf(ctx, "Skipping VMMMS verification because kernel version is %q", version.String())
		return nil
	}
	resourcedClientOK := false
	resourcedServerOK := false
	lmkdClientOK := false
	mglruClientOK := false

	for !(resourcedClientOK && resourcedServerOK && lmkdClientOK && mglruClientOK) {
		entry, err := iv.reader.Read()
		if err != nil {
			return errors.Wrap(err, "failed to scan syslog when checking VMMMS status")
		}
		// Resourced logs when VMMMS has been successfully initialized.
		if entry.Program == "resourced" && entry.Content == "Enabling VmMemoryManagementClient" {
			resourcedClientOK = true
		}
		if entry.Program == "vm_concierge" {
			if strings.Contains(entry.Content, "Server rejecting connection from un-registered VM") {
				return errors.New("server rejected a connection from an un-registered VM")
			}
			// Parse server side connection logs.
			match := vmmmsServerRE.FindStringSubmatch(entry.Content)
			if match == nil {
				continue
			}
			// Resourced uses a loopback connection with CID 1.
			if match[1] == "1" && match[2] == "CONNECTION_TYPE_KILLS" {
				resourcedServerOK = true
			}
			// ARCVM will have a CID not equal to 1.
			if match[1] != "1" && match[2] == "CONNECTION_TYPE_KILLS" {
				lmkdClientOK = true
			}
			if match[1] != "1" && match[2] == "CONNECTION_TYPE_STATS" {
				mglruClientOK = true
			}
		}
	}
	return nil
}
