package plutigo

import (
	"context"
	"testing"
	"time"

	"github.com/Salvionied/apollo/v2/backend"
)

// TestResolveSlotStateAppliesShelleyAnchor verifies that the slot->time
// conversion uses the Shelley era anchor for networks with a Byron era, rather
// than naively anchoring absolute slot 0 at the Byron system start. Getting
// this wrong shifts every converted time by the accumulated Byron offset (~18.5
// days on preprod), which silently breaks any Plutus script that compares its
// validity-range bound against a POSIX timestamp.
func TestResolveSlotStateAppliesShelleyAnchor(t *testing.T) {
	cases := []struct {
		name             string
		byronSystemStart int64
		slot             uint64
		// wantUnixMilli is the POSIX time (ms) the slot must convert to, matching
		// the canonical off-chain slot<->time mapping used by the ledger/Ogmios.
		wantUnixMilli int64
	}{
		{
			// preprod: Shelley begins at slot 86400 anchored at 1655769600 (not
			// the Byron system start 1654041600). Verified against a live preprod
			// block: slot 126554038 == unix 1782237238.
			name:             "preprod",
			byronSystemStart: 1654041600,
			slot:             126551926,
			wantUnixMilli:    1782235126000,
		},
		{
			// mainnet: Shelley began at slot 4492800 / 1596059091.
			name:             "mainnet",
			byronSystemStart: 1506203091,
			slot:             4492800,
			wantUnixMilli:    1596059091000,
		},
		{
			// preview has no Byron era, so the legacy genesis-derived anchor
			// (slot 0 at the system start) is already correct.
			name:             "preview",
			byronSystemStart: 1666656000,
			slot:             100,
			wantUnixMilli:    1666656100000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			genesis := backend.GenesisParameters{
				SystemStart: tc.byronSystemStart,
				SlotLength:  1,
			}
			provider, err := New(Config{GenesisParamsOverride: &genesis})
			if err != nil {
				t.Fatalf("New failed: %v", err)
			}
			slotState, err := provider.resolveSlotState(context.Background())
			if err != nil {
				t.Fatalf("resolveSlotState failed: %v", err)
			}
			got, err := slotState.SlotToTime(tc.slot)
			if err != nil {
				t.Fatalf("SlotToTime failed: %v", err)
			}
			if got.UnixMilli() != tc.wantUnixMilli {
				t.Fatalf(
					"slot %d converted to %d ms (%s), want %d ms (%s)",
					tc.slot,
					got.UnixMilli(),
					got.UTC(),
					tc.wantUnixMilli,
					time.UnixMilli(tc.wantUnixMilli).UTC(),
				)
			}
		})
	}
}
