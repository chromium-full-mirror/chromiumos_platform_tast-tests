// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featured

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"

	featuredpb "chromiumos/system_api/featured_proto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	dutStorePath = "/var/lib/featured/store"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StoreInterfaceEarlyBoot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify data store exists after featured restarts and boot attempts field incremented",
		Contacts: []string{
			"cros-telemetry@google.com",
			"kendraketsui@google.com",
			"mutexlox@google.com",
		},
		BugComponent: "b:1096648", // ChromeOS > Data > Engineering > Featured
		SoftwareDeps: []string{"reboot"},
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      60 * time.Second,
	})
}

func StoreInterfaceEarlyBoot(ctx context.Context, s *testing.State) {
	d := s.DUT()
	hostStoreBeforeReboot := filepath.Join(s.OutDir(), "original-store")

	// Copy store.
	if err := linuxssh.GetFile(ctx, d.Conn(), dutStorePath, hostStoreBeforeReboot, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy store file to Host: ", err)
	}

	// Read store.
	data, err := os.ReadFile(hostStoreBeforeReboot)
	if err != nil {
		s.Fatal("Failed to read store: ", err)
	}

	// Deserialize store and make var for the boot counter.
	store := &featuredpb.Store{}
	if err = proto.Unmarshal(data, store); err != nil {
		s.Fatal("Failed to parse store: ", err)
	}
	bootAttempts := store.GetBootAttemptsSinceLastSeedUpdate()

	// Restart featured.
	s.Log("Rebooting DUT")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// Wait until featured finishes restarting.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		hostStoreAfterReboot := filepath.Join(s.OutDir(), "updated-store")

		// Copy store.
		if err = linuxssh.GetFile(ctx, d.Conn(), dutStorePath, hostStoreAfterReboot, linuxssh.PreserveSymlinks); err != nil {
			return errors.Wrap(err, "failed to copy store from DUT after restart")
		}

		// Read store.
		data, err = os.ReadFile(hostStoreAfterReboot)
		if err != nil {
			return errors.Wrap(err, "failed to read store after restart")
		}

		// Deserialize new store.
		if err = proto.Unmarshal(data, store); err != nil {
			return errors.Wrap(err, "failed to parse store after restart")
		}
		updatedBootAttempts := store.GetBootAttemptsSinceLastSeedUpdate()

		// Break out of poll if boots is larger or smaller than expected value.
		if updatedBootAttempts > bootAttempts+1 || updatedBootAttempts < bootAttempts {
			return testing.PollBreak(errors.Errorf("boot attempts counter did not increment correctly after restart. Original value: %d. Updated value: %d", bootAttempts, updatedBootAttempts))
		}

		// Check that boots is counter + 1.
		if updatedBootAttempts != bootAttempts+1 {
			return errors.Errorf("boot attempts counter did not increment after restart. Original value: %d. Updated value: %d", bootAttempts, updatedBootAttempts)
		}

		return nil

	}, &testing.PollOptions{
		Timeout: 30 * time.Second,
	}); err != nil {
		s.Fatal("Failed to wait for counter to increment: ", err)
	}
}
