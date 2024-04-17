// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Ehide, or Ethernet-hide, is a tool that creates an environment that hides
// the Ethernet interface on DUT but still allows ssh to it through that Ethernet
// interface. Ehide only runs on test images. For details please check
// go/cros-ehide.

// Package ehide contains util functions for the ehide tool. The util functions
// are mainly about ehide state checking.
package ehide
