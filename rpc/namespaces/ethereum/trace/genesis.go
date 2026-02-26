package trace

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	dtracer "github.com/cosmos/evm/debank/tracer"
	dtypes "github.com/cosmos/evm/debank/types"
	rpctypes "github.com/cosmos/evm/rpc/types"
	erc20types "github.com/cosmos/evm/x/erc20/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/cosmos/gogoproto/jsonpb"
	"github.com/cosmos/gogoproto/proto"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

//go:embed genesis.json
var genesisState string

type AppState = map[string]json.RawMessage

func appState() (AppState, error) {
	var genesis map[string]json.RawMessage
	err := json.Unmarshal([]byte(genesisState), &genesis)
	if err != nil {
		return nil, err
	}
	var state AppState
	err = json.Unmarshal(genesis["app_state"], &state)
	if err != nil {
		panic(err)
	}
	return state, nil
}

func extractState(appState AppState, module string, val proto.Message) error {
	evmGenesis := appState[module]
	buf := bytes.NewBuffer(evmGenesis)
	err := jsonpb.Unmarshal(buf, val)
	if err != nil {
		return err
	}
	return nil
}
func evmState(appState AppState) (evmtypes.GenesisState, error) {
	var state evmtypes.GenesisState
	err := extractState(appState, "evm", &state)
	if err != nil {
		return evmtypes.GenesisState{}, err
	}
	return state, nil
}

func erc20State(appState AppState) (erc20types.GenesisState, error) {
	var state erc20types.GenesisState
	err := extractState(appState, "erc20", &state)
	if err != nil {
		return erc20types.GenesisState{}, err
	}
	return state, nil
}

func bankState(appState AppState) (banktypes.GenesisState, error) {
	var state banktypes.GenesisState
	err := extractState(appState, "bank", &state)
	if err != nil {
		return banktypes.GenesisState{}, err
	}
	return state, nil
}

func genutilState(appState AppState) (genutiltypes.GenesisState, error) {
	var state genutiltypes.GenesisState
	err := extractState(appState, "genutil", &state)
	if err != nil {
		return genutiltypes.GenesisState{}, err
	}
	return state, nil
}

func collectTxs(state genutiltypes.GenesisState) ([]Tx, error) {
	txs := make([]Tx, 0, len(state.GenTxs))
	for _, genTx := range state.GenTxs {
		var tx Tx
		if err := json.Unmarshal(genTx, &tx); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

func bech32ToAddress(bech32Addr string) (common.Address, error) {
	_, bz, err := bech32.DecodeAndConvert(bech32Addr)
	if err != nil {
		return common.Address{}, err
	}
	hexStr := hex.EncodeToString(bz)
	return common.HexToAddress("0x" + hexStr), nil
}

func (api API) genesisAllocToStateDiff(appState AppState) (*dtypes.BlockStorageDiff, error) {
	diff := &dtypes.BlockStorageDiff{
		Hash:            common.Hash{},
		ParentHash:      common.Hash{},
		NewAccounts:     make([]dtypes.NewAccount, 0),
		DeletedAccounts: make([]common.Hash, 0),
		StorageDiff:     make([]dtypes.AccountStorageDiff, 0),
		NewCodes:        make([]dtypes.NewCode, 0),
	}
	evmGenesisState, err := evmState(appState)
	if err != nil {
		return nil, err
	}
	bankGenesisState, err := bankState(appState)
	if err != nil {
		return nil, err
	}
	erc20GenesisState, err := erc20State(appState)
	if err != nil {
		return nil, err
	}
	genutilGenesisState, err := genutilState(appState)
	if err != nil {
		return nil, err
	}
	txs, err := collectTxs(genutilGenesisState)
	if err != nil {
		return nil, err
	}
	var (
		number                 = rpctypes.BlockNumber(1)
		bankGenesisAddress     = make([]common.Address, 0)
		genTxsValidatorAddress = make([]common.Address, 0)
		blockOrHash            = rpctypes.BlockNumberOrHash{BlockNumber: &number}
	)
	for _, balance := range bankGenesisState.Balances {
		bech32Addr, err := bech32ToAddress(balance.Address)
		if err != nil {
			return nil, err
		}
		bankGenesisAddress = append(bankGenesisAddress, bech32Addr)
	}
	for _, tx := range txs {
		address, err := bech32ToAddress(tx.Body.Messages[0].ValidatorAddress)
		if err != nil {
			return nil, err
		}
		genTxsValidatorAddress = append(genTxsValidatorAddress, address)
	}

	for _, erc20Addr := range erc20GenesisState.NativePrecompiles {
		address := common.HexToAddress(erc20Addr)
		balance, err := api.backend.GetBalance(address, blockOrHash)
		if err != nil {
			return nil, err
		}
		nonce, err := api.backend.GetTransactionCount(address, number)
		if err != nil {
			return nil, err
		}
		code, err := api.backend.GetCode(address, blockOrHash)
		if err != nil {
			return nil, err
		}
		codeHash := crypto.Keccak256Hash(code)
		diff.NewAccounts = append(diff.NewAccounts, dtypes.NewAccount{
			Address:  crypto.Keccak256Hash(address.Bytes()[:]),
			Balance:  uint256.MustFromBig((*big.Int)(balance)),
			Nonce:    uint64(*nonce),
			CodeHash: codeHash,
		})
		diff.NewCodes = append(diff.NewCodes, dtypes.NewCode{
			CodeHash: codeHash,
			Code:     code,
		})
	}

	for _, account := range evmGenesisState.Accounts {
		address := common.HexToAddress(account.Address)
		balance, err := api.backend.GetBalance(address, blockOrHash)
		if err != nil {
			return nil, err
		}
		nonce, err := api.backend.GetTransactionCount(address, number)
		if err != nil {
			return nil, err
		}
		addressHash := crypto.Keccak256Hash(address.Bytes())
		code := common.Hex2Bytes(account.Code)
		codeHash := crypto.Keccak256Hash(code)
		diff.NewAccounts = append(diff.NewAccounts, dtypes.NewAccount{
			Address:  addressHash,
			Balance:  uint256.MustFromBig((*big.Int)(balance)),
			Nonce:    uint64(*nonce),
			CodeHash: codeHash,
		})
		if len(account.Code) > 0 {
			diff.NewCodes = append(diff.NewCodes, dtypes.NewCode{
				CodeHash: codeHash,
				Code:     code,
			})
		}
		values := make([]dtypes.IndexValuePair, 0)
		for _, state := range account.Storage {
			key := common.HexToHash(state.Key)
			v := common.HexToHash(state.Value)
			value := uint256.NewInt(0).SetBytes(v.Bytes())
			values = append(values, dtypes.IndexValuePair{
				Index: crypto.Keccak256Hash(key[:]),
				Value: value,
			})
		}
		diff.StorageDiff = append(diff.StorageDiff, dtypes.AccountStorageDiff{
			Address: addressHash,
			Values:  values,
		})
	}
	for _, address := range append(bankGenesisAddress, genTxsValidatorAddress...) {
		balance, err := api.backend.GetBalance(address, rpctypes.BlockNumberOrHash{BlockNumber: &number})
		if err != nil {
			return nil, err
		}
		nonce, err := api.backend.GetTransactionCount(address, number)
		if err != nil {
			return nil, err
		}
		diff.NewAccounts = append(diff.NewAccounts, dtypes.NewAccount{
			Address:  crypto.Keccak256Hash(address.Bytes()[:]),
			Balance:  uint256.MustFromBig((*big.Int)(balance)),
			Nonce:    uint64(*nonce),
			CodeHash: crypto.Keccak256Hash(nil),
		})
	}
	return diff, nil
}

func (api API) onGenesisBlock(block map[string]interface{}) (*dtypes.DebankOutPut, error) {
	appState, err := appState()
	if err != nil {
		return nil, err
	}
	header := dtracer.BuildPilelineBlockHeader(block)
	blockDiff, err := api.genesisAllocToStateDiff(appState)
	if err != nil {
		return nil, err
	}
	blockDiff.Hash = header.StateRoot
	blockDiff.ParentHash = ethtypes.EmptyRootHash

	blockFile := &dtypes.BlockFile{
		Block:            dtracer.BuildPipelineBlock(block),
		Txs:              make([]dtypes.Transaction, 0),
		Events:           make([]dtypes.Event, 0),
		Traces:           make([]dtypes.Trace, 0),
		ErrorEvents:      make([]dtypes.Event, 0),
		ErrorTraces:      make([]dtypes.Trace, 0),
		StorageContracts: make([]string, 0),
	}

	evmstate, _ := evmState(appState)
	for _, acc := range evmstate.Accounts {
		if len(acc.Storage) > 0 {
			blockFile.StorageContracts = append(blockFile.StorageContracts, strings.ToLower(acc.Address))
		}
	}
	return &dtypes.DebankOutPut{
		BlockFile:      blockFile,
		Header:         header,
		StateDiff:      blockDiff,
		ValidationHash: blockFile.Validation().ValidationHash,
	}, nil
}
