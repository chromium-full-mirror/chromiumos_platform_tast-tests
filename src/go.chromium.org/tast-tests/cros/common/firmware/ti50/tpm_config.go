// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"math/rand"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func envTestSeed() (int64, bool) {
	if v := os.Getenv("TPM_TEST_SEED"); v != "" {
		if s, err := strconv.ParseInt(v, 10, 64); err == nil {
			return s, true
		}
	}
	return 0, false
}

func defaultTestSeed() int64 {
	if s, ok := envTestSeed(); ok {
		return s
	}
	return time.Now().UnixNano()
}

// TestRand wraps a pseudo-random generator with convenience helpers for
// generating and mutating test payloads.
type TestRand struct {
	Seed int64
	*rand.Rand
}

// NewTestRand returns a new TestRand seeded with seed.
func NewTestRand(seed int64) *TestRand {
	return &TestRand{
		Seed: seed,
		Rand: rand.New(rand.NewSource(seed)),
	}
}

func (r *TestRand) ensureRand() *rand.Rand {
	if r == nil {
		return rand.New(rand.NewSource(1))
	}
	if r.Rand == nil {
		if r.Seed == 0 {
			r.Seed = defaultTestSeed()
		}
		r.Rand = rand.New(rand.NewSource(r.Seed))
	}
	return r.Rand
}

// RandomBytes generates n pseudo-random bytes.
func (r *TestRand) RandomBytes(n int) []byte {
	if n <= 0 {
		return nil
	}
	b := make([]byte, n)
	_, _ = r.ensureRand().Read(b)
	return b
}

// RandomInt returns a pseudo-random int in [minVal, maxVal].
func (r *TestRand) RandomInt(minVal, maxVal int) int {
	if maxVal <= minVal {
		return minVal
	}
	return minVal + r.ensureRand().Intn(maxVal-minVal+1)
}

// ModifyBytes returns a copy of src with at least one byte mutated.
func (r *TestRand) ModifyBytes(src []byte) []byte {
	if len(src) == 0 {
		return []byte{0xFF}
	}
	out := append([]byte(nil), src...)
	idx := r.ensureRand().Intn(len(out))
	out[idx] ^= 0xFF
	return out
}

// TpmConfig holds capabilities, supported commands, algorithms, ECC curves,
// and properties discovered from the TPM under test.
type TpmConfig struct {
	// Commands maps supported TPM command codes to true.
	Commands map[tpm2.TPMCC]bool
	// Algs maps supported TPM algorithm IDs to true.
	Algs map[tpm2.TPMAlgID]bool
	// AlgProperties maps supported TPM algorithm IDs to their attribute flags.
	AlgProperties map[tpm2.TPMAlgID]tpm2.TPMAAlgorithm
	// HashAlgs lists all pure hash algorithms supported by both the TPM and Go crypto.
	HashAlgs []tpm2.TPMAlgID
	// OaepHashAlgs lists supported hash algorithms whose digest size is at most
	// 32 bytes. On GSC firmware, sizeof(TPMT_HA) is 34 bytes (2-byte hashAlg +
	// 32-byte digest), which caps RSA-OAEP and salt-encryption hash digests at
	// 32 bytes even when SHA-384/SHA-512 are enabled for standalone hashing.
	OaepHashAlgs []tpm2.TPMAlgID
	// Properties maps fixed TPM property tags (PT_FIXED TPM_PT_*) to their
	// 32-bit values.
	Properties map[tpm2.TPMPT]uint32
	// ECCCurveList lists all supported ECC curve IDs in reported order.
	ECCCurveList []tpm2.TPMECCCurve
	// RsaKeySizes lists supported RSA key sizes (in bits) discovered via TPM2_TestParms.
	RsaKeySizes []tpm2.TPMKeyBits
	// SymDefs lists supported symmetric cipher algorithm/keyBits/mode combinations
	// discovered via TPM2_TestParms.
	SymDefs []tpm2.TPMTSymDefObject
	// PCRBanks lists allocated PCR bank selections reported by TPM_CAP_PCRS.
	PCRBanks []tpm2.TPMSPCRSelection
}

func (t *TpmHandle) logDiscoveryError(format string, args ...any) {
	if t != nil && t.Ctx != nil {
		testing.ContextLogf(t.Ctx, format, args...)
	}
}

// InitTestConfig discovers the target TPM configuration via TPM2_GetCapability
// and ensures the pseudo-random generator on t is initialized and logged.
func (t *TpmHandle) InitTestConfig() error {
	cfg := &TpmConfig{
		Commands:      make(map[tpm2.TPMCC]bool),
		Algs:          make(map[tpm2.TPMAlgID]bool),
		AlgProperties: make(map[tpm2.TPMAlgID]tpm2.TPMAAlgorithm),
		Properties:    make(map[tpm2.TPMPT]uint32),
	}

	// 1. Discover supported commands.
	nextCmd := uint32(tpm2.TPMCCNVUndefineSpaceSpecial)
	for {
		getCapability := tpm2.GetCapability{
			Capability:    tpm2.TPMCapCommands,
			Property:      nextCmd,
			PropertyCount: 64,
		}
		resp, err := getCapability.Execute(t)
		if err != nil {
			t.logDiscoveryError("TPM_CAP_COMMANDS query failed at 0x%x: %v", nextCmd, err)
			return errors.Wrap(err, "failed to query TPM_CAP_COMMANDS")
		}
		cmds, err := resp.CapabilityData.Data.Command()
		if err != nil {
			t.logDiscoveryError("TPM_CAP_COMMANDS unpack failed at 0x%x: %v", nextCmd, err)
			return errors.Wrap(err, "failed to unpack TPM_CAP_COMMANDS data")
		}
		if len(cmds.CommandAttributes) == 0 {
			break
		}
		var lastCC uint32
		for _, attr := range cmds.CommandAttributes {
			cc := tpm2.TPMCC(attr.CommandIndex)
			if attr.V {
				cc |= 1 << 29
			}
			cfg.Commands[cc] = true
			lastCC = uint32(cc)
		}
		if !resp.MoreData || lastCC < nextCmd {
			break
		}
		nextCmd = lastCC + 1
	}

	// 2. Discover supported algorithms.
	nextAlg := uint32(0)
	for {
		getCapability := tpm2.GetCapability{
			Capability:    tpm2.TPMCapAlgs,
			Property:      nextAlg,
			PropertyCount: 64,
		}
		resp, err := getCapability.Execute(t)
		if err != nil {
			t.logDiscoveryError("TPM_CAP_ALGS query failed at 0x%x: %v", nextAlg, err)
			return errors.Wrap(err, "failed to query TPM_CAP_ALGS")
		}
		algs, err := resp.CapabilityData.Data.Algorithms()
		if err != nil {
			t.logDiscoveryError("TPM_CAP_ALGS unpack failed at 0x%x: %v", nextAlg, err)
			return errors.Wrap(err, "failed to unpack TPM_CAP_ALGS data")
		}
		if len(algs.AlgProperties) == 0 {
			break
		}
		var lastAlg uint32
		for _, ap := range algs.AlgProperties {
			cfg.Algs[ap.Alg] = true
			cfg.AlgProperties[ap.Alg] = ap.AlgProperties
			if isPureHashAttribute(ap.AlgProperties) && HashSize(ap.Alg) > 0 {
				cfg.HashAlgs = append(cfg.HashAlgs, ap.Alg)
				// GSC firmware builds ms-tpm-20-ref with MAX_DIGEST_SIZE == 32
				// (sizeof(TPMT_HA) == 34), so RSA-OAEP and salt-encryption hashes
				// cannot exceed 32-byte digests.
				if HashSize(ap.Alg) <= 32 {
					cfg.OaepHashAlgs = append(cfg.OaepHashAlgs, ap.Alg)
				}
			}
			lastAlg = uint32(ap.Alg)
		}
		if !resp.MoreData || lastAlg < nextAlg {
			break
		}
		nextAlg = lastAlg + 1
	}
	if len(cfg.HashAlgs) == 0 {
		return errors.New("TPM reported no supported pure hash algorithms")
	}

	// 3. Discover fixed (PT_FIXED) TPM properties. Variable (PT_VAR) properties
	// change during a test, so they are not cached and must be queried with
	// GetTPMProperty when needed.
	nextProp := uint32(tpm2.TPMPTFamilyIndicator)
propLoop:
	for {
		getCapability := tpm2.GetCapability{
			Capability:    tpm2.TPMCapTPMProperties,
			Property:      nextProp,
			PropertyCount: 64,
		}
		resp, err := getCapability.Execute(t)
		if err != nil {
			t.logDiscoveryError("TPM_CAP_TPM_PROPERTIES query failed at 0x%x: %v", nextProp, err)
			return errors.Wrap(err, "failed to query TPM_CAP_TPM_PROPERTIES")
		}
		props, err := resp.CapabilityData.Data.TPMProperties()
		if err != nil {
			t.logDiscoveryError("TPM_CAP_TPM_PROPERTIES unpack failed at 0x%x: %v", nextProp, err)
			return errors.Wrap(err, "failed to unpack TPM_CAP_TPM_PROPERTIES data")
		}
		if len(props.TPMProperty) == 0 {
			break
		}
		var lastProp uint32
		for _, p := range props.TPMProperty {
			if p.Property >= tpm2.TPMPTPermanent {
				break propLoop
			}
			cfg.Properties[p.Property] = p.Value
			lastProp = uint32(p.Property)
		}
		if !resp.MoreData || lastProp < nextProp {
			break
		}
		nextProp = lastProp + 1
	}
	if cfg.Properties[tpm2.TPMPTMaxDigest] == 0 {
		return errors.New("TPM did not report a non-zero TPM_PT_MAX_DIGEST property")
	}

	// 4. Discover supported ECC curves (if ECC is supported).
	if cfg.Algs[tpm2.TPMAlgECC] {
		nextCurve := uint32(0)
		for {
			getCapability := tpm2.GetCapability{
				Capability:    tpm2.TPMCapECCCurves,
				Property:      nextCurve,
				PropertyCount: 64,
			}
			resp, err := getCapability.Execute(t)
			if err != nil {
				t.logDiscoveryError("TPM_CAP_ECC_CURVES query failed at 0x%x: %v", nextCurve, err)
				if nextCurve == 0 {
					return errors.Wrap(err, "failed to query TPM_CAP_ECC_CURVES")
				}
				break
			}
			curves, err := resp.CapabilityData.Data.ECCCurves()
			if err != nil {
				t.logDiscoveryError("TPM_CAP_ECC_CURVES unpack failed at 0x%x: %v", nextCurve, err)
				return errors.Wrap(err, "failed to unpack TPM_CAP_ECC_CURVES data")
			}
			if len(curves.ECCCurves) == 0 {
				break
			}
			var lastCurve uint32
			for _, c := range curves.ECCCurves {
				if !slices.Contains(cfg.ECCCurveList, c) {
					cfg.ECCCurveList = append(cfg.ECCCurveList, c)
				}
				lastCurve = uint32(c)
			}
			if !resp.MoreData || lastCurve < nextCurve {
				break
			}
			nextCurve = lastCurve + 1
		}
	}

	// 5. Discover supported RSA key sizes and symmetric cipher definitions via TPM2_TestParms.
	if cfg.Algs[tpm2.TPMAlgRSA] {
		if cfg.Commands[tpm2.TPMCCTestParms] {
			for _, bits := range []tpm2.TPMKeyBits{1024, 1536, 2048, 2560, 3072, 3584, 4096} {
				testParms := tpm2.TestParms{
					Parameters: tpm2.TPMTPublicParms{
						Type: tpm2.TPMAlgRSA,
						Parameters: tpm2.NewTPMUPublicParms(
							tpm2.TPMAlgRSA,
							&tpm2.TPMSRSAParms{
								Symmetric: tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
								Scheme:    tpm2.TPMTRSAScheme{Scheme: tpm2.TPMAlgNull},
								KeyBits:   bits,
								Exponent:  0,
							},
						),
					},
				}
				if _, tpErr := testParms.Execute(t); tpErr == nil {
					cfg.RsaKeySizes = append(cfg.RsaKeySizes, bits)
				}
			}
			if len(cfg.RsaKeySizes) == 0 {
				return errors.New("TPM supports TPM_ALG_RSA but TPM2_TestParms rejected all probed RSA key sizes")
			}
		} else {
			cfg.RsaKeySizes = []tpm2.TPMKeyBits{2048}
		}
	}

	if cfg.Algs[tpm2.TPMAlgSymCipher] {
		symAlgs := []tpm2.TPMAlgID{tpm2.TPMAlgAES, tpm2.TPMAlgSM4, tpm2.TPMAlgTDES}
		symKeySizes := []tpm2.TPMKeyBits{128, 192, 256}
		symModes := []tpm2.TPMAlgID{
			tpm2.TPMAlgCTR,
			tpm2.TPMAlgOFB,
			tpm2.TPMAlgCBC,
			tpm2.TPMAlgCFB,
			tpm2.TPMAlgECB,
		}
		for _, alg := range symAlgs {
			if !cfg.Algs[alg] {
				continue
			}
			for _, bits := range symKeySizes {
				for _, mode := range symModes {
					if !cfg.Algs[mode] {
						continue
					}
					symDef := tpm2.TPMTSymDefObject{
						Algorithm: alg,
						KeyBits:   tpm2.NewTPMUSymKeyBits(alg, bits),
						Mode:      tpm2.NewTPMUSymMode(alg, mode),
					}
					if cfg.Commands[tpm2.TPMCCTestParms] {
						testParms := tpm2.TestParms{
							Parameters: tpm2.TPMTPublicParms{
								Type: tpm2.TPMAlgSymCipher,
								Parameters: tpm2.NewTPMUPublicParms(
									tpm2.TPMAlgSymCipher,
									&tpm2.TPMSSymCipherParms{Sym: symDef},
								),
							},
						}
						if _, tpErr := testParms.Execute(t); tpErr != nil {
							continue
						}
					}
					cfg.SymDefs = append(cfg.SymDefs, symDef)
				}
			}
		}
	}

	// 6. Discover allocated PCR banks.
	getCapability := tpm2.GetCapability{
		Capability:    tpm2.TPMCapPCRs,
		Property:      0,
		PropertyCount: 64,
	}
	pcrResp, err := getCapability.Execute(t)
	if err != nil {
		t.logDiscoveryError("TPM_CAP_PCRS query failed: %v", err)
		return errors.Wrap(err, "failed to query TPM_CAP_PCRS")
	}
	assignedPCRs, err := pcrResp.CapabilityData.Data.AssignedPCR()
	if err != nil {
		t.logDiscoveryError("TPM_CAP_PCRS unpack failed: %v", err)
		return errors.Wrap(err, "failed to unpack TPM_CAP_PCRS data")
	}
	cfg.PCRBanks = assignedPCRs.PCRSelections

	t.Config = cfg
	if s, ok := envTestSeed(); ok {
		t.Rand = NewTestRand(s)
	} else if t.Rand == nil {
		t.Rand = NewTestRand(defaultTestSeed())
	} else {
		_ = t.Rand.ensureRand()
	}
	t.logDiscoveryError("TPM test PRNG seed: %d", t.Rand.Seed)
	return nil
}

func isPureHashAttribute(attr tpm2.TPMAAlgorithm) bool {
	return attr.Hash &&
		!attr.Asymmetric &&
		!attr.Symmetric &&
		!attr.Object &&
		!attr.Signing &&
		!attr.Encrypting &&
		!attr.Method
}

// IsCommandSupported reports whether the TPM supports the given command code.
func (c *TpmConfig) IsCommandSupported(cc tpm2.TPMCC) bool {
	return c != nil && c.Commands[cc]
}

// IsAlgSupported reports whether the TPM supports the given algorithm ID.
func (c *TpmConfig) IsAlgSupported(alg tpm2.TPMAlgID) bool {
	return c != nil && c.Algs[alg]
}

// IsHashAlg reports whether alg is a pure hash algorithm supported by the TPM.
func (c *TpmConfig) IsHashAlg(alg tpm2.TPMAlgID) bool {
	if c == nil {
		return false
	}
	for _, h := range c.HashAlgs {
		if h == alg {
			return true
		}
	}
	return false
}

// IsCurveSupported reports whether the TPM supports the given ECC curve.
func (c *TpmConfig) IsCurveSupported(curve tpm2.TPMECCCurve) bool {
	return c != nil && slices.Contains(c.ECCCurveList, curve)
}

// Property returns the value of the requested fixed TPM property (or 0 if not
// present). Variable (PT_VAR) properties are not cached; use GetTPMProperty.
func (c *TpmConfig) Property(pt tpm2.TPMPT) uint32 {
	if c == nil {
		return 0
	}
	return c.Properties[pt]
}

// MaxNVBufferSize returns TPM_PT_NV_BUFFER_MAX, falling back to TPM_PT_MAX_DIGEST
// or defaultMaxNVBufferSize (512) if TPM_PT_NV_BUFFER_MAX is 0.
func (c *TpmConfig) MaxNVBufferSize() uint32 {
	if v := c.Property(tpm2.TPMPTNVBufferMax); v > 0 {
		return v
	}
	if d := c.Property(tpm2.TPMPTMaxDigest); d > 0 {
		return d
	}
	return uint32(defaultMaxNVBufferSize)
}

// DefaultHashAlg returns the preferred general-purpose hash algorithm
// (preferring SHA-256 if supported, or the first supported hash algorithm).
func (c *TpmConfig) DefaultHashAlg() tpm2.TPMAlgID {
	if c != nil {
		if slices.Contains(c.HashAlgs, tpm2.TPMAlgSHA256) {
			return tpm2.TPMAlgSHA256
		}
		if len(c.HashAlgs) > 0 {
			return c.HashAlgs[0]
		}
	}
	return tpm2.TPMAlgSHA256
}

// DefaultOaepHashAlg returns the preferred OAEP hash algorithm (preferring
// SHA-256 if supported, or the first supported OAEP/hash algorithm).
func (c *TpmConfig) DefaultOaepHashAlg() tpm2.TPMAlgID {
	if c != nil {
		if slices.Contains(c.OaepHashAlgs, tpm2.TPMAlgSHA256) {
			return tpm2.TPMAlgSHA256
		}
		if len(c.OaepHashAlgs) > 0 {
			return c.OaepHashAlgs[0]
		}
		if len(c.HashAlgs) > 0 {
			return c.HashAlgs[0]
		}
	}
	return tpm2.TPMAlgSHA256
}

// RandomHashAlg returns a pseudo-random supported hash algorithm from
// t.Config.HashAlgs.
func (t *TpmHandle) RandomHashAlg() tpm2.TPMAlgID {
	return t.Config.HashAlgs[t.Rand.RandomInt(0, len(t.Config.HashAlgs)-1)]
}

// AltHashAlg returns the first supported hash algorithm in c.HashAlgs that
// differs from exclude, or exclude itself if only one hash algorithm is
// supported.
func (c *TpmConfig) AltHashAlg(exclude tpm2.TPMAlgID) tpm2.TPMAlgID {
	if c != nil {
		for _, alg := range c.HashAlgs {
			if alg != exclude {
				return alg
			}
		}
	}
	return exclude
}

// ActivePCRBankHash returns the hash algorithm of the first allocated PCR bank
// in c.PCRBanks (non-zero PCRSelect and supported hash algorithm), falling back
// to TPM_ALG_SHA256.
func (c *TpmConfig) ActivePCRBankHash() tpm2.TPMAlgID {
	if c != nil {
		for _, bank := range c.PCRBanks {
			if c.IsHashAlg(bank.Hash) && slices.ContainsFunc(bank.PCRSelect, func(b byte) bool { return b != 0 }) {
				return bank.Hash
			}
		}
	}
	return tpm2.TPMAlgSHA256
}
