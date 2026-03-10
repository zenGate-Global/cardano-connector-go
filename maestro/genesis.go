package maestro

import (
	"fmt"
	"strings"

	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
)

var genesisPresetsByNetwork = map[string]Base.GenesisParameters{
	"mainnet": {
		ActiveSlotsCoefficient: 0.05,
		UpdateQuorum:           5,
		MaxLovelaceSupply:      "45000000000000000",
		NetworkMagic:           764824073,
		EpochLength:            432000,
		SystemStart:            1506203091,
		SlotsPerKesPeriod:      129600,
		SlotLength:             1,
		MaxKesEvolutions:       62,
		SecurityParam:          2160,
	},
	"preprod": {
		ActiveSlotsCoefficient: 0.05,
		UpdateQuorum:           5,
		MaxLovelaceSupply:      "45000000000000000",
		NetworkMagic:           1,
		EpochLength:            432000,
		SystemStart:            1654041600,
		SlotsPerKesPeriod:      129600,
		SlotLength:             1,
		MaxKesEvolutions:       62,
		SecurityParam:          2160,
	},
	"preview": {
		ActiveSlotsCoefficient: 0.05,
		UpdateQuorum:           5,
		MaxLovelaceSupply:      "45000000000000000",
		NetworkMagic:           2,
		EpochLength:            86400,
		SystemStart:            1666656000,
		SlotsPerKesPeriod:      129600,
		SlotLength:             1,
		MaxKesEvolutions:       62,
		SecurityParam:          432,
	},
}

func resolveGenesisParams(config Config, networkName string) (Base.GenesisParameters, error) {
	if config.GenesisParamsOverride != nil {
		return *config.GenesisParamsOverride, nil
	}

	preset, ok := genesisPresetsByNetwork[strings.ToLower(networkName)]
	if !ok {
		return Base.GenesisParameters{}, fmt.Errorf(
			"unsupported or missing network name: %s",
			config.NetworkName,
		)
	}

	return preset, nil
}
