// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var intelPCITransitionIterationsVar = testing.RegisterVarString(
	"intel.PCITransition.iterations",
	"10",
	"The number of iterations to run the PCI transition check",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PCITransitionCheck,
		Desc:         "Check the PCI transition from L0 to L1",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com", "sangram.k.y@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-nda"},
		Fixture:      "chromeLoggedIn",
		Timeout:      7 * time.Minute,
	})
}

// PCITransitionCheck checks the PCI transition from L0 to L1.
func PCITransitionCheck(ctx context.Context, s *testing.State) {
	const lspciCmd = "lspci | awk '{print $1}'"
	devices := strings.Fields(cmdOutput(ctx, s, lspciCmd))

	varValue := intelPCITransitionIterationsVar.Value()
	iterations, err := strconv.Atoi(varValue)
	if err != nil || iterations <= 0 {
		s.Fatalf("Failed to parse iterations value %v: %v", varValue, err)
	}

	for i := 1; i <= iterations; i++ {
		s.Logf("Iteration: %d", i)
		for _, dev := range devices {
			s.Logf("Checking device %s", dev)
			if dev == "" {
				continue
			}

			if _, err := os.Stat("/sys/bus/pci/devices/" + dev); os.IsNotExist(err) {
				dev = "0000:" + dev
			}

			deviceTransitionCheck(ctx, s, dev)
		}
	}
}

func deviceTransitionCheck(ctx context.Context, s *testing.State, pciAddress string) {
	// Skip the NVMe / UFS devices.
	pciClassCode, err := getPCIClassCode(ctx, s, pciAddress)
	if err != nil {
		s.Log("PCI Class code not found")
		return
	}
	if strings.HasPrefix(pciClassCode, "01") {
		s.Logf("Skipping storage device at %s", pciAddress)
		return
	}

	// Find the Base Address of the PCI Express Capability Structure.
	baseInt, err := findCapBaseAdd(ctx, s, pciAddress)
	if err != nil {
		s.Log("Failed to find the base address of the PCI express: ", err)
		return
	}

	// Read the Link Status Register (offset 0x12 from the PCI Express Capability structure).
	regAddress := fmt.Sprintf("0x%X.W", baseInt+0x12)
	linkStatus := setPCI(ctx, s, pciAddress, regAddress)
	if linkStatus == "" {
		s.Errorf("Failed to read Link Status Register for device %s", pciAddress)
		return
	}

	// Read the Link Control Register (offset 0x10 from the PCI Express Capability structure).
	regAddress = fmt.Sprintf("0x%X.W", baseInt+0x10)
	linkControl := setPCI(ctx, s, pciAddress, regAddress)
	if linkControl == "" {
		s.Errorf("Failed to read Link Control Register for device %s", pciAddress)
		return
	}

	s.Logf("PCI Address: %s", pciAddress)
	s.Logf("Link Status Register: 0x%s", linkStatus)
	s.Logf("Link Control Register: 0x%s", linkControl)

	// Decode the Link Control Register to determine the link state.
	printCurrentLinkStatus(ctx, s, linkControl)

	// Read the L1 Substates Control 1 Register (offset 0x18 from the PCI Express Capability structure).
	regAddress = fmt.Sprintf("0x%X.W", baseInt+0x18)
	l1SubstatesControl1 := setPCI(ctx, s, pciAddress, regAddress)
	if l1SubstatesControl1 == "" {
		s.Errorf("Failed to read L1 Substates Control 1 Register for device %s", pciAddress)
		return
	}

	// Convert the l1SubstatesControl1 string to an integer.
	l1SubstatesControl1Int, err := strconv.ParseInt(l1SubstatesControl1, 16, 64)
	if err != nil {
		s.Error("Failed to parse link control: ", err)
		return
	}

	// Decode the L1 Substates Control 1 Register.
	l1_1Enabled := int64(0x01 & l1SubstatesControl1Int)
	l1_2Enabled := int64((0x01 & (l1SubstatesControl1Int >> 1)))

	if l1_1Enabled == 1 {
		s.Log("L1.1 Substate: Enabled")
	} else {
		s.Log("L1.1 Substate: Disabled")
	}

	if l1_2Enabled == 1 {
		s.Log("L1.2 Substate: Enabled")
	} else {
		s.Log("L1.2 Substate: Disabled")
	}

	// Read the L1 Substates Control 2 Register (offset 0x1A from the PCI Express Capability structure)
	regAddress = fmt.Sprintf("0x%X.W", baseInt+0x1A)
	l1SubstatesControl2 := setPCI(ctx, s, pciAddress, regAddress)
	if l1SubstatesControl2 == "" {
		s.Errorf("Failed to read L1 Substates Control 2 Register for device %s", pciAddress)
		return
	}

	// Convert the l1SubstatesControl2 string to an integer.
	l1SubstatesControl2Int, err := strconv.ParseInt(l1SubstatesControl2, 16, 64)
	if err != nil {
		s.Error("Failed to parse link control: ", err)
		return
	}

	// Decode the L1 Substates Control 2 Register
	commonModeRestoreTime := int64(0xFF & l1SubstatesControl2Int)
	tPowerOnScale := int64((0x03 & (l1SubstatesControl2Int >> 8)))
	tPowerOnValue := int64((0x3F & (l1SubstatesControl2Int >> 10)))

	s.Logf("Common Mode Restore Time: %d", commonModeRestoreTime)
	s.Logf("T_Power_On Scale: %d", tPowerOnScale)
	s.Logf("T_Power_On Value: %d", tPowerOnValue)

	// Transitions between L0 and L1 states
	printCurrentLinkStatus(ctx, s, linkControl)
	s.Logf("Transitioning from L0 to L1 for device %s", pciAddress)
	setPCI(ctx, s, pciAddress, fmt.Sprintf("CAP_EXP+0x10.W=0x2"))

	// GoBigSleepLint: Let it transition from L0 to L1.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		s.Error("Failed to sleep: ", err)
	}

	// Verify transition to L1
	linkControl = setPCI(ctx, s, pciAddress, "CAP_EXP+0x10.W")
	s.Logf("Link Control Register after transition to L1: 0x%s", linkControl)
	// Convert the l1SubstatesControl1 string to an integer.
	linkControlInt, err := strconv.ParseInt(linkControl, 16, 64)
	if err != nil {
		s.Error("Failed to parse link control: ", err)
		return
	}

	newLinkControlDec := int64(linkControlInt)
	if (newLinkControlDec & 0x2) == 0x2 {
		s.Logf("Successfully transitioned to L1 for device %s", pciAddress)
	} else {
		s.Errorf("Failed to transition to L1 for device %s", pciAddress)
		return
	}
	printCurrentLinkStatus(ctx, s, linkControl)

	s.Logf("Transitioning back from L1 to L0 for device %s", pciAddress)
	setPCI(ctx, s, pciAddress, fmt.Sprintf("CAP_EXP+0x10.W=0x0"))
	// GoBigSleepLint: Let it transition from L1 to L0.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		s.Error("Failed to sleep: ", err)
	}

	// Verify transition to L0
	linkControl = setPCI(ctx, s, pciAddress, "CAP_EXP+0x10.W")
	// Convert the l1SubstatesControl1 string to an integer.
	linkControlInt, err = strconv.ParseInt(linkControl, 16, 64)
	if err != nil {
		s.Error("Failed to parse link control: ", err)
		return
	}
	newLinkControlDec = int64(linkControlInt)
	s.Logf("Link Control Register after L1 to L0: 0x%s", linkControl)
	if (newLinkControlDec & 0x2) == 0 {
		s.Logf("Successfully transitioned to L0 for device %s", pciAddress)
	} else {
		s.Errorf("Failed to transition to L0 for device %s", pciAddress)
	}
	printCurrentLinkStatus(ctx, s, linkControl)
}

func setPCI(ctx context.Context, s *testing.State, pciAddress, regAddress string) string {
	cmd := fmt.Sprintf("setpci -s %s %s", pciAddress, regAddress)
	output := cmdOutput(ctx, s, cmd)
	return strings.TrimSpace(output)
}

func printCurrentLinkStatus(ctx context.Context, s *testing.State, linkControl string) {
	aspmControl, err := strconv.ParseInt(linkControl, 16, 64)
	if err != nil {
		s.Log("Error parsing link control: ", err)
		return
	}
	aspmControl &= 0x3

	switch aspmControl {
	case 0:
		s.Log("Link State: L0 (Active)")
	case 1:
		s.Log("Link State: L0s (ASPM L0s)")
	case 2:
		s.Log("Link State: L1 (ASPM L1)")
	case 3:
		s.Log("Link State: L0s and L1 (ASPM L0s and L1)")
	default:
		s.Log("Link State: Unknown")
	}
}

func findCapBaseAdd(ctx context.Context, s *testing.State, pciAddress string) (int64, error) {
	cmd := fmt.Sprintf("lspci -vv -s %s | grep -m 1 -i 'Capabilities:.*Express' | awk '{print $2}' | sed 's/\\[//;s/\\]//'", pciAddress)
	output := strings.TrimSpace(cmdOutput(ctx, s, cmd))
	if output == "" {
		return 0, errors.Errorf("Unable to find PCI Express Capability structure for device %s", pciAddress)
	}

	// Convert the base address string to an Hex.
	baseInt, err := strconv.ParseInt(output, 16, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert base address to Hex")
	}
	return baseInt, nil
}

func getPCIClassCode(ctx context.Context, s *testing.State, pciAddress string) (string, error) {
	cmd := fmt.Sprintf("lspci -s %s -nn", pciAddress)
	output := cmdOutput(ctx, s, cmd)
	re := regexp.MustCompile(`\[(\w{2})\d*]:`)
	match := re.FindStringSubmatch(strings.TrimSpace(output))
	if len(match) > 1 {
		return match[1], nil
	}
	return "", errors.New("match not found")
}

// cmdOutput executes CPU commands and returns output.
func cmdOutput(ctx context.Context, s *testing.State, cmd string) string {
	out, err := testexec.CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to exceute %q command: %v", cmd, err)
	}
	return string(out)
}
