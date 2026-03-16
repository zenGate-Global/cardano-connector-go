module github.com/zenGate-Global/cardano-connector-go

go 1.24.0

require (
	github.com/Salvionied/apollo v1.1.1
	github.com/Salvionied/cbor/v2 v2.6.0
	github.com/SundaeSwap-finance/kugo v1.3.0
	github.com/SundaeSwap-finance/ogmigo/v6 v6.1.0
	github.com/blinklabs-io/gouroboros v0.160.0
	github.com/blinklabs-io/plutigo v0.0.26
	github.com/gorilla/websocket v1.5.3
	github.com/maestro-org/go-sdk v1.2.1
	github.com/stretchr/testify v1.11.1
	github.com/tj/assert v0.0.3
	github.com/utxorpc/go-codegen v0.18.1
	github.com/utxorpc/go-sdk v0.0.3
)

require (
	connectrpc.com/connect v1.19.1 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime v0.0.0-20251001021608-1fe7b43fc4d6 // indirect
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/bits-and-blooms/bitset v1.20.0 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.6 // indirect
	github.com/btcsuite/btcd/btcutil v1.1.6 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0 // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/consensys/gnark-crypto v0.19.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0 // indirect
	github.com/ethereum/go-ethereum v1.17.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.3 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/SundaeSwap-finance/ogmigo/v6 => github.com/mgpai22/ogmigo/v6 v6.0.4

// replace github.com/SundaeSwap-finance/kugo => github.com/mgpai22/kugo v1.2.1

// replace github.com/utxorpc/go-sdk => github.com/mgpai22/go-sdk v0.0.3

replace github.com/maestro-org/go-sdk => github.com/mgpai22/maestro-cardano-go-sdk v0.0.0-20260309092003-ccf2793f8d4d
