// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"bufio"
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Microcode,
		Desc: "Checks that compatible CPU microcode is built into the kernel",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		// TODO(b/254348748): move out of informational when brya issues are resolved.
		Attr: []string{"group:mainline", "informational"},
		// TODO(crbug.com/1092389): This test only knows how to check Intel platforms
		// right now. Ideally it would be restricted with a HardwareDep to Intel SoCs
		// only. The respective HardwareDep requires some preparation work though, see
		// crbug.com/1092389 and crbug.com/1094802. For the time being, restrict the test
		// to "amd64" (which incorrectly includes AMD platforms as well, on which the
		// test will trivially pass).
		SoftwareDeps: []string{"microcode", "amd64"},
	})
}

func Microcode(ctx context.Context, s *testing.State) {
	vmlinuz, err := os.CreateTemp("/tmp", "vmlinuz")
	defer os.Remove(vmlinuz.Name())
	err = readKernelImage(ctx, vmlinuz)
	if err != nil {
		s.Fatal("Failed to read kernel image: ", err)
	}

	vmlinux, err := unpackKernelImage(ctx, vmlinuz)
	if err != nil {
		s.Fatal("Failed to extract kernel image: ", err)
	}

	fwmap, err := extractBuiltinFirmware(vmlinux)
	if err != nil {
		s.Fatal("Failed to extract builtin firmware: ", err)
	}

	for name, fw := range fwmap {
		s.Logf("Built-in firmware %s (%d bytes)", name, len(fw))
	}

	cpuinfo, err := readCPUInfo()
	if err != nil {
		s.Fatal("Failed to read CPU info: ", err)
	}

	for _, cpu := range cpuinfo {
		if cpu.Vendor != "GenuineIntel" {
			continue
		}

		id := fmt.Sprintf("%02x-%02x-%02x", cpu.Family, cpu.Model, cpu.Stepping)
		fwname := fmt.Sprintf("intel-ucode/%s", id)

		microcode, ok := fwmap[fwname]
		if !ok {
			// Test failure here indicates that no microcode that matches the processor
			// ID is bundled in the kernel. Make sure that cros-kernel.eclass includes
			// the appropriate microcode in CONFIG_EXTRA_FIRMWARE.
			s.Errorf("No built-in microcode for id %s", id)
			continue
		}

		pf, err := readSysfsCPUVal(cpu.Index, "microcode/processor_flags")
		if err != nil {
			s.Fatal("Failed to read processor flags: ", err)
		}

		rev, err := readSysfsCPUVal(cpu.Index, "microcode/version")
		if err != nil {
			s.Fatal("Failed to read microcode revision: ", err)
		}

		s.Logf("CPU %d id %s pf %#02x rev %#02x", cpu.Index, id, pf, rev)

		microcodeRev := uint32(0)
		hdrs, err := parseMicrocodeHeaders(microcode)
		if err != nil {
			s.Fatal("Failed to parse microcode headers: ", err)
		}
		for _, header := range hdrs {
			// This is a simplified check for whether microcode is compatible. In
			// particular, it doesn't take into account the trailing header that may
			// list alternative and processor ids flags. Logic to check that can be
			// added if we ever need it.
			if (uint64(header.Pf) & pf) != 0 {
				if header.Rev > microcodeRev {
					microcodeRev = header.Rev
				}
			}
		}

		if microcodeRev == 0 {
			// Test failure here indicates that while microcode for the processor ID is
			// present, it doesn't appear compatible with the CPU. To fix this, make
			// sure the correct microcode gets built into the kernel image by
			// cros-kernel.eclass. If correct microcode appears to be present and get
			// loaded correctly, this may be because a bug in the test code, e.g.
			// because of disagreement between test test code and the kernel a microcode
			// image is compatible.
			s.Errorf("Microcode for id %s not compatible with pf %#02x", id, pf)
		}

		// Tolerate cases where the running microcode is a different, but more later
		// revision than the bundled one. This can happen if the CPU ships with updated
		// microcode or if firmware includes fresher microcode than the kernel.
		if rev < uint64(microcodeRev) {
			// If we fail here, then we found microcode in the kernel image that looks
			// compatible in the but it is not running on the CPU. This could be caused
			// by a corrupt microcode binary.
			s.Errorf("Microcode rev mismatch: %#02x < %#02x", rev, microcodeRev)
		}
	}
}

// readKernelImage obtains the kernel image from the booted kernel partition.
// The kernel image is written to the temp file 'vmlinuz', so it can be unpacked
// (decompressed) later.
func readKernelImage(ctx context.Context, vmlinuz *os.File) error {
	dev, err := getKernelPartition(ctx)
	if err != nil {
		return err
	}

	_, err = testexec.CommandContext(ctx,
		"futility",
		"vbutil_kernel",
		"--get-vmlinuz",
		dev,
		"--vmlinuz-out",
		vmlinuz.Name()).Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}

	return nil
}

// Matches the partition number on a block device name.
var rePartition = regexp.MustCompile("[0-9]+$")

// getKernelPartition determines the booted kernel partition device name from rootdev output.
func getKernelPartition(ctx context.Context) (string, error) {
	dev, err := testexec.CommandContext(ctx, "rootdev", "-s").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}

	err = nil
	kdev := rePartition.ReplaceAllStringFunc(
		strings.TrimSpace(string(dev)),
		func(match string) string {
			val, innerErr := strconv.ParseUint(match, 0, 64)
			if innerErr != nil {
				err = innerErr
				return ""
			}
			return strconv.FormatUint(val-1, 10)
		})

	if err != nil {
		return "", err
	}

	return kdev, nil
}

// extractVmlinux unpacks (decompresses) the linux kernel in the 'vmlinuz' file.
// This function executes a script that attempts various decompression algorithms based
// on magic bytes being present in the file. Only minimal checking on the output file
// is possible (looking for "ELF" in the file description) due to limitations on tools
// available on the DUT (i.e., 'readelf' is not present).
func extractVmlinux(ctx context.Context, vmlinuz *os.File) ([]byte, error) {
	// The following script is adapted (and heavily modified) from:
	//   kernel/v6.6/scripts/extract-vmlinux
	// This handles any file compression that the linux kernel is able to, as of v6.6.
	extractVmlinuxSh := `
try_decompress()
{
	# 1. for pos in $(tr "$1\n$2" "\n$2=" < "$img" | grep -abo "^$2")
	#   - Find all offsets of the specific magic characters we're looking for.
	#     - Example output:
	#       $ tr '\135\0\0\0'\nxxx \nxxx= < vmlinuz | grep -abo "^xxx"
	#       110:xxx
	#       1076:xxx
	#       2637:xxx
	# 2. pos=${pos%%:*}
	#   - Remove the colon and everything after it from that output.
	#     - Example output:
	#       $ pos=110:xxx ; pos=${pos%%:*} ; echo $pos
	#       110
	# 3. tail -c+$pos "$img" | $3 > $tmp 2> /dev/null;
	#   - Send vmlinuz contents from that offset onward into the chosen decompression program.
	# 4. file $tmp | grep -s ELF 2>&1 &&  exit 0
	#   - Check the decompressed file contains "ELF" in the description.
	#   - If vmlinuz fails to be decompressed successfully, 'file' will not output a valid
	#   description.
	for	pos in $(tr "$1\n$2" "\n$2=" < "$img" | grep -abo "^$2")
	do
		pos=${pos%%:*}
		tail -c+$pos "$img" | $3 > $tmp 2> /dev/null
		file $tmp | grep -s ELF 2>&1 &&  exit 0
	done
}

img="$0"
tmp="$1"

try_decompress '\037\213\010' xy    gunzip
try_decompress '\3757zXZ\000' abcde unxz
try_decompress 'BZh'          xy    bunzip2
try_decompress '\135\0\0\0'   xxx   unlzma
try_decompress '\211\114\132' xy    'lzop -d'
try_decompress '\002!L\030'   xxx   'lz4 -d'
try_decompress '(\265/\375'   xxx   unzstd

echo "Cannot find vmlinux." >&2
exit 1
`

	extractedVmlinux, err := os.CreateTemp("/tmp", "vmlinux")
	defer os.Remove(extractedVmlinux.Name())
	_, err = testexec.CommandContext(
		ctx,
		"bash",
		"-c",
		extractVmlinuxSh,
		vmlinuz.Name(),
		extractedVmlinux.Name()).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed extract vmlinux")
	}

	vmlinuxBytes, err := os.ReadFile(extractedVmlinux.Name())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read uncompressed linux path: %q", extractedVmlinux.Name())
	}

	return vmlinuxBytes, nil
}

// unpackKernelImage decompresses the compressed kernel image.
func unpackKernelImage(ctx context.Context, vmlinuz *os.File) ([]byte, error) {
	vmlinux, err := extractVmlinux(ctx, vmlinuz)
	if err != nil {
		return nil, err
	}

	return vmlinux, nil
}

type fwentry struct {
	Name uint64
	Addr uint64
	Size uint64
}

// extractBuiltinFirmware parses a kernel image and extracts a map of built-in firmware blobs. The
// .builtin_fw section in the image contains (name, location, size) triples, where name and location
// reference memory in the .rodata section. See the build commands for CONFIG_EXTRA_FIRMWARE in the
// kernel tree for details.
func extractBuiltinFirmware(vmlinux []byte) (map[string][]byte, error) {
	f, err := elf.NewFile(bytes.NewReader(vmlinux))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fws := f.Section(".builtin_fw")
	if fws == nil {
		return nil, errors.New("no .builtin_fw section")
	}
	fwsr := fws.Open()

	ros := f.Section(".rodata")
	if ros == nil {
		return nil, errors.New("no .rodata section")
	}
	ror := ros.Open()

	m := make(map[string][]byte)
	for {
		var fwe fwentry
		err := binary.Read(fwsr, binary.LittleEndian, &fwe)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		_, err = ror.Seek(int64(fwe.Name-ros.Addr), io.SeekStart)
		if err != nil {
			return nil, err
		}

		name, err := bufio.NewReader(ror).ReadString(0)
		if err != nil {
			return nil, err
		}
		name = strings.TrimRight(name, "\x00")

		_, err = ror.Seek(int64(fwe.Addr-ros.Addr), io.SeekStart)
		if err != nil {
			return nil, err
		}

		fw := make([]byte, fwe.Size)
		n, err := ror.Read(fw)
		if err != nil {
			return nil, err
		}
		if uint64(n) != fwe.Size {
			return nil, errors.New("Short built-in firmware read")
		}

		m[name] = fw
	}

	return m, nil
}

type cpuInfo struct {
	Index    uint   `cpuinfo:"processor"`
	Vendor   string `cpuinfo:"vendor_id"`
	Family   uint   `cpuinfo:"cpu family"`
	Model    uint   `cpuinfo:"model"`
	Stepping uint   `cpuinfo:"stepping"`
}

// readCPUInfo parses /proc/cpuinfo into a map of cpuInfo objects.
func readCPUInfo() ([]cpuInfo, error) {
	cpuinfo, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil {
		return nil, err
	}

	cpus := strings.Split(string(cpuinfo), "\n\n")
	r := make([]cpuInfo, len(cpus))
	for i, cpu := range cpus {
		if strings.TrimSpace(cpu) == "" {
			continue
		}

		m := map[string]string{}
		for _, line := range strings.Split(cpu, "\n") {
			c := strings.Split(line, ":")
			if len(c) == 2 {
				key := strings.TrimSpace(c[0])
				value := strings.TrimSpace(c[1])
				m[key] = value
			}
		}

		st := reflect.TypeOf(r[i])
		obj := reflect.ValueOf(&r[i]).Elem()
		for i := 0; i < st.NumField(); i++ {
			tag := st.Field(i).Tag.Get("cpuinfo")
			value, ok := m[tag]
			if !ok {
				return nil, errors.Errorf("Missing value for cpuinfo key %s", tag)
			}

			field := obj.Field(i)
			kind := field.Kind()
			switch kind {
			case reflect.String:
				field.SetString(value)
			case reflect.Uint:
				n, err := strconv.ParseUint(value, 0, 64)
				if err != nil {
					return nil, err
				}
				field.SetUint(n)
			}
		}
	}

	return r, nil
}

func readSysfsCPUVal(index uint, name string) (uint64, error) {
	path := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/%s", index, name)

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}

	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 0, 64)
	if err != nil {
		return 0, err
	}

	return v, nil
}

type microcodeHeader struct {
	Hdrver    uint32
	Rev       uint32
	Date      uint32
	Sig       uint32
	Cksum     uint32
	Ldrver    uint32
	Pf        uint32
	Datasize  uint32
	Totalsize uint32
	Reserved  [3]uint32
}

func parseMicrocodeHeaders(microcode []byte) ([]microcodeHeader, error) {
	var r []microcodeHeader
	rdr := bytes.NewReader(microcode)
	for {
		var hdr microcodeHeader
		err := binary.Read(rdr, binary.LittleEndian, &hdr)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		_, err = rdr.Seek(int64(hdr.Totalsize)-int64(binary.Size(hdr)), io.SeekCurrent)
		if err != nil {
			return nil, err
		}

		r = append(r, hdr)
	}

	return r, nil
}
