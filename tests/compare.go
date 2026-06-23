package tests

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// UtxosEqual reports whether two gouroboros UTxOs are SEMANTICALLY equal.
//
// reflect.DeepEqual (and ==) are unreliable for common.Utxo: the concrete
// output types embed cbor.DecodeStoreCbor (a cached raw-CBOR byte slice) and
// hold values behind interfaces, so two UTxOs that represent the exact same
// on-chain output compare unequal when one was decoded from the wire and the
// other built by hand (as the fixtures are). This helper compares the
// observable fields instead: input (txhash#index), address, lovelace, native
// assets, datum, and reference script.
func UtxosEqual(a, b common.Utxo) bool {
	return utxoDiff(a, b) == ""
}

// UtxoDiff returns a human-readable description of the first semantic
// difference between two UTxOs, or "" if they are equal. Useful in test
// failure messages.
func UtxoDiff(a, b common.Utxo) string {
	return utxoDiff(a, b)
}

func utxoDiff(a, b common.Utxo) string {
	// Input: tx hash + index.
	if !bytes.Equal(a.Id.Id().Bytes(), b.Id.Id().Bytes()) {
		return fmt.Sprintf("tx hash: %x != %x", a.Id.Id().Bytes(), b.Id.Id().Bytes())
	}
	if a.Id.Index() != b.Id.Index() {
		return fmt.Sprintf("index: %d != %d", a.Id.Index(), b.Id.Index())
	}

	ao, bo := a.Output, b.Output
	if (ao == nil) != (bo == nil) {
		return "one output is nil"
	}
	if ao == nil {
		return ""
	}

	// Address (bech32).
	aAddr, bAddr := ao.Address(), bo.Address()
	if aAddr.String() != bAddr.String() {
		return fmt.Sprintf("address: %s != %s", aAddr.String(), bAddr.String())
	}

	// Lovelace.
	aAmt, bAmt := ao.Amount(), bo.Amount()
	if (aAmt == nil) != (bAmt == nil) {
		return "one lovelace amount is nil"
	}
	if aAmt != nil && aAmt.Cmp(bAmt) != 0 {
		return fmt.Sprintf("lovelace: %s != %s", aAmt.String(), bAmt.String())
	}

	// Native assets.
	if diff := assetsDiff(ao, bo); diff != "" {
		return diff
	}

	// Datum (inline) and datum hash.
	if diff := datumDiff(ao, bo); diff != "" {
		return diff
	}

	// Reference script.
	if diff := scriptRefDiff(ao, bo); diff != "" {
		return diff
	}

	return ""
}

func assetsDiff(ao, bo common.TransactionOutput) string {
	aAssets, bAssets := ao.Assets(), bo.Assets()
	if (aAssets == nil) != (bAssets == nil) {
		return "one assets set is nil"
	}
	if aAssets == nil {
		return ""
	}

	type entry struct {
		policy string
		name   string
		qty    string
	}
	flatten := func(ma *common.MultiAsset[common.MultiAssetTypeOutput]) []entry {
		var out []entry
		for _, policy := range ma.Policies() {
			for _, name := range ma.Assets(policy) {
				out = append(out, entry{
					policy: policy.String(),
					name:   string(name),
					qty:    ma.Asset(policy, name).String(),
				})
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].policy != out[j].policy {
				return out[i].policy < out[j].policy
			}
			return out[i].name < out[j].name
		})
		return out
	}

	af, bf := flatten(aAssets), flatten(bAssets)
	if len(af) != len(bf) {
		return fmt.Sprintf("asset count: %d != %d", len(af), len(bf))
	}
	for i := range af {
		if af[i] != bf[i] {
			return fmt.Sprintf("asset[%d]: %+v != %+v", i, af[i], bf[i])
		}
	}
	return ""
}

func datumDiff(ao, bo common.TransactionOutput) string {
	aDatum, bDatum := ao.Datum(), bo.Datum()
	if (aDatum == nil) != (bDatum == nil) {
		return "one inline datum is nil"
	}
	if aDatum != nil {
		aBytes, aErr := aDatum.MarshalCBOR()
		bBytes, bErr := bDatum.MarshalCBOR()
		if aErr != nil || bErr != nil {
			return fmt.Sprintf("datum marshal error: %v / %v", aErr, bErr)
		}
		if !bytes.Equal(aBytes, bBytes) {
			return fmt.Sprintf("inline datum: %x != %x", aBytes, bBytes)
		}
	}

	aHash, bHash := ao.DatumHash(), bo.DatumHash()
	if (aHash == nil) != (bHash == nil) {
		return "one datum hash is nil"
	}
	if aHash != nil && *aHash != *bHash {
		return fmt.Sprintf("datum hash: %x != %x", aHash.Bytes(), bHash.Bytes())
	}
	return ""
}

func scriptRefDiff(ao, bo common.TransactionOutput) string {
	aScript, bScript := ao.ScriptRef(), bo.ScriptRef()
	if (aScript == nil) != (bScript == nil) {
		return "one reference script is nil"
	}
	if aScript != nil {
		if !bytes.Equal(aScript.Hash().Bytes(), bScript.Hash().Bytes()) {
			return fmt.Sprintf("reference script hash: %x != %x",
				aScript.Hash().Bytes(), bScript.Hash().Bytes())
		}
	}
	return ""
}

// RedeemersApproxEqual reports whether two redeemer ExUnits maps have the same
// set of keys and ExUnits within tolerancePct of each other. Live script
// evaluation drifts slightly over time as cost models change, so integration
// tests assert approximate ExUnits rather than exact values. tolerancePct is a
// fraction (e.g. 0.02 for +/-2%).
func RedeemersApproxEqual(
	got, want map[common.RedeemerKey]common.ExUnits,
	tolerancePct float64,
) (bool, string) {
	if len(got) != len(want) {
		return false, fmt.Sprintf("redeemer count: got %d, want %d", len(got), len(want))
	}
	for key, wantEU := range want {
		gotEU, ok := got[key]
		if !ok {
			return false, fmt.Sprintf("missing redeemer key %+v", key)
		}
		if !withinTolerance(gotEU.Memory, wantEU.Memory, tolerancePct) {
			return false, fmt.Sprintf("redeemer %+v memory: got %d, want ~%d (+/-%.0f%%)",
				key, gotEU.Memory, wantEU.Memory, tolerancePct*100)
		}
		if !withinTolerance(gotEU.Steps, wantEU.Steps, tolerancePct) {
			return false, fmt.Sprintf("redeemer %+v steps: got %d, want ~%d (+/-%.0f%%)",
				key, gotEU.Steps, wantEU.Steps, tolerancePct*100)
		}
	}
	return true, ""
}

func withinTolerance(got, want int64, tolerancePct float64) bool {
	if want == 0 {
		return got == 0
	}
	diff := float64(got-want) / float64(want)
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerancePct
}
