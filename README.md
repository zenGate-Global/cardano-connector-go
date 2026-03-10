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

## Local transaction evaluation with Plutigo

The `plutigo` package adds a `connector.Provider` wrapper for local transaction evaluation.

- `EvaluateTx` runs locally with `gouroboros` and `plutigo`.
- All other `Provider` methods still delegate to the wrapped provider.
- `additionalUTxOs` are preferred over fetched chain data.
- The wrapped provider is used only to fetch any missing prerequisites for local eval:
  - spent/reference inputs
  - datums
  - scripts
  - protocol parameters
  - genesis / slot timing data

This means the normal integration pattern is:

- use `blockfrost`, `kupmios`, `maestro`, or `utxorpc` as your normal provider
- wrap it with `plutigo`
- pass the wrapped provider anywhere your code expects `connector.Provider`
- keep reads and submission remote
- run only script evaluation locally

### Common usage

```go
base, err := blockfrost.New(blockfrost.Config{
    ProjectID: "<blockfrost-project-id>",
    BaseURL:   "https://cardano-preview.blockfrost.io/api/v0",
    NetworkId: 0,
})
if err != nil {
    panic(err)
}

provider, err := plutigo.Wrap(base)
if err != nil {
    panic(err)
}

// Use provider normally everywhere else:
utxos, err := provider.GetUtxosByAddress(ctx, addr)
txHash, err := provider.SubmitTx(ctx, signedTx)

// Only EvaluateTx is local:
exUnits, err := provider.EvaluateTx(ctx, txCbor, additionalUTxOs)
```

### How this is typically used in an app

If your transaction builder or wallet layer already accepts `connector.Provider`, no builder changes are needed. Wrap the provider once at construction time:

```go
base, err := blockfrost.New(blockfrost.Config{
    ProjectID: "<project-id>",
    BaseURL:   "https://cardano-preview.blockfrost.io/api/v0",
    NetworkId: 0,
})
if err != nil {
    panic(err)
}

provider := connector.Provider(base)
if cfg.LocalTxEval {
    provider, err = plutigo.Wrap(base)
    if err != nil {
        panic(err)
    }
}

// Pass provider into your tx builder / Apollo chain context / service layer.
```

That is the pattern used successfully in `merkle-oracle-node`: the wrapped provider was passed into the existing transaction flow, and Apollo continued calling `EvaluateTx(...)` through the same `connector.Provider` interface.

### When to use `Wrap(...)` vs `New(...)`

Use `Wrap(...)` for the normal case:

```go
provider, err := plutigo.Wrap(base)
```

Use `New(...)` only when the wrapped provider does not expose enough data for local eval and you need to supply overrides:

```go
provider, err := plutigo.New(plutigo.Config{
    Provider:               base,
    ProtocolParamsOverride: protocolParams,
    GenesisParamsOverride:  genesisParams,
    SlotConfig:             slotConfig,
})
```

`SlotConfig` is useful when you know the correct network slot timing but your provider does not expose a usable genesis endpoint.

### Provider notes

- `blockfrost`
  - validated for local eval in a real transaction flow
  - good default choice for rollout
- `kupmios`
  - good fit in principle because it already has strong eval-related chain data
- `maestro`
  - can be wrapped, but local eval still depends on Maestro exposing enough protocol/genesis data
  - if not, use `New(...)` with overrides
- `utxorpc`
  - same caveat as Maestro for slot/genesis/protocol prerequisites

### Important behavior

- This wrapper does not make your whole provider local.
- It only changes script evaluation.
- Reads like `GetUtxosByAddress`, `GetDatum`, `GetScriptCborByScriptHash`, `GetTip`, and `SubmitTx` still use the wrapped provider.
- If local eval fails because required chain metadata is unavailable, either:
  - choose a provider that exposes it, or
  - pass explicit overrides with `plutigo.New(...)`

### Minimal eval-only example

```go
base, err := blockfrost.New(blockfrost.Config{
    ProjectID: "<blockfrost-project-id>",
    BaseURL:   "https://cardano-preview.blockfrost.io/api/v0",
    NetworkId: 0,
})
if err != nil {
    panic(err)
}

localEval, err := plutigo.Wrap(base)
if err != nil {
    panic(err)
}

exUnits, err := localEval.EvaluateTx(ctx, txCbor, additionalUTxOs)
```

## Maestro genesis presets

The Maestro provider includes hardcoded genesis presets for `mainnet`, `preprod`, and `preview` so `GetGenesisParams()` works even though Maestro does not expose a full genesis endpoint.

If those network values ever change before this library is updated, pass `GenesisParamsOverride` in `maestro.Config` to override the built-in preset.

The Maestro provider also includes hardcoded scalar protocol-parameter defaults for fields its SDK does not expose in the older `Base.ProtocolParameters` shape used by this repo. If those values ever change before this library is updated, pass `ProtocolParamsOverride` in `maestro.Config` to replace the built-in defaults entirely.
