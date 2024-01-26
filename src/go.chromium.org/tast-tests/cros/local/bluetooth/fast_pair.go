// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

// Fast pair notification IDs.
const (
	// NotificationIDFastPairDiscoveryGuest is the ID of the fast pair discovery
	// notification when signed in as a guest.
	NotificationIDFastPairDiscoveryGuest = "cros_fast_pair_discovery_guest_notification_id"

	// NotificationIDFastPairDiscoveryUser is the ID of the fast pair discovery
	// notification when signed in as a user.
	NotificationIDFastPairDiscoveryUser = "cros_fast_pair_discovery_user_notification_id"

	// NotificationIDFastPairSubsequentUser is the ID of the fast pair subsequent
	// pair notification. It only shows up for signed in users.
	NotificationIDFastPairSubsequentPair = "cros_fast_pair_discovery_subsequent_notification_id"

	// NotificationIDFastPairRetroactivePair is the ID of the fast pair retroactive
	// pair notification. It only shows up for signed in users.
	NotificationIDFastPairRetroactivePair = "cros_fast_pair_associate_account_notification_id"

	// NotificationIDFastPairPairing is the ID of the fast pair notification
	// displayed when pairing is in progress.
	NotificationIDFastPairPairing = "cros_fast_pair_pairing_notification_id"

	// NotificationIDFastPairError is the ID of the fast pair notification
	// displayed when a pairing error has occurred.
	NotificationIDFastPairError = "cros_fast_pair_error_notification_id"
)
