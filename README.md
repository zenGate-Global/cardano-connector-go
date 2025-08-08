# Cardano Connector Go

A Go library that provides a unified interface for interacting with multiple Cardano blockchain API providers, simplifying integration and reducing vendor lock-in.

## Interface Overview

The library defines a `Provider` interface that standardizes access to essential Cardano blockchain operations:

### Core Functionality

**Protocol & Network**

- `GetProtocolParameters()` - Fetch current network protocol parameters
- `SubmitTx()` - Submit signed transactions to the network
- `AwaitTx()` - Wait for transaction confirmation with configurable polling

**UTxO Management**

- `GetUtxosByAddress()` - Query UTxOs by Bech32 address
- `GetUtxosWithUnit()` - Filter UTxOs by specific asset units
- `GetUtxoByUnit()` - Find UTxO containing a specific token/NFT
- `GetUtxosByOutRef()` - Query UTxOs by transaction output references

**Smart Contracts & Data**

- `GetDatum()` - Retrieve datum by hash as PlutusData
- `EvaluateTx()` - Evaluate transaction scripts and calculate execution units

**Staking**

- `GetDelegation()` - Get delegation info and rewards for stake addresses

## Implementation Status

### Completed Providers

- [x] **Blockfrost**
- [x] **Kupmios**
- [x] **UTxORPC**
- [x] **Maestro**

### Providers to Implement

- [ ] **Koios**

## TODO

- kupmios eval not completly working, see test2, test3 for eval
- utxorpc eval not working, unable to find fully functional api providers atm
- plutus script bytes -> plutus script type matching needs to be implemented, its harded coded to v2 atm
- make sure maestro protocol params are all filled as much as possible
