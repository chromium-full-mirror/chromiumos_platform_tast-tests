// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featured

import (
	"context"
	"os"
	"time"

	"github.com/golang/protobuf/proto"

	featuredpb "chromiumos/system_api/featured_proto"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

const (
	storePath = "/var/lib/featured/store"
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
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      60 * time.Second,
	})
}

func StoreInterfaceEarlyBoot(ctx context.Context, s *testing.State) {
	// Read store.
	data, err := os.ReadFile(storePath)
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
	if err := upstart.RestartJob(ctx, "featured"); err != nil {
		s.Fatal("Failed to restart featured: ", err)
	}

	// Wait until featured finishes restarting.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Read store.
		data, err = os.ReadFile(storePath)
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
