package tracer

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	dtypes "github.com/cosmos/evm/debank/types"
	"github.com/cosmos/evm/x/vm/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

func BuildPipelineBlock(rawBlock map[string]interface{}) dtypes.Block {
	block := dtypes.Block{
		ID:                    rawHash(rawBlock["hash"]).Hex(),
		Height:                rawBig(rawBlock["number"]),
		ParentID:              rawBlock["parentHash"].(common.Hash).Hex(),
		BaseFeePerGas:         big.NewInt(0),
		Miner:                 strings.ToLower(rawBlock["miner"].(common.Address).Hex()),
		GasLimit:              new(big.Int).SetUint64(rawUint64(rawBlock["gasLimit"])),
		GasUsed:               rawBig(rawBlock["gasUsed"]),
		Timestamp:             rawUint64(rawBlock["timestamp"]),
		ProcessStartTimestamp: time.Now().UnixMilli(),
	}
	if baseFeePerGas, ok := rawBlock["baseFeePerGas"]; ok {
		block.BaseFeePerGas = rawBig(baseFeePerGas)
	}
	return block
}

func BuildPipelineTransaction(
	tx *ethtypes.Transaction,
	index int64,
	from common.Address,
	gasUsed *big.Int,
	baseFee *big.Int,
	success bool,
) dtypes.Transaction {
	var to = common.Address{}
	if tx.To() != nil {
		to = *tx.To()
	}
	transaction := dtypes.Transaction{
		ID:               tx.Hash().Hex(),
		From:             strings.ToLower(from.Hex()),
		To:               strings.ToLower(to.Hex()),
		Gas:              big.NewInt(int64(tx.Gas())),
		GasUsed:          gasUsed,
		GasPrice:         tx.GasPrice(),
		Status:           success,
		GasFeeCap:        common.Big0,
		GasTipCap:        common.Big0,
		Input:            tx.Data(),
		Nonce:            big.NewInt(int64(tx.Nonce())),
		TransactionIndex: index,
		Value:            (*hexutil.Big)(tx.Value()),
	}
	switch tx.Type() {
	case ethtypes.DynamicFeeTxType:
		transaction.GasFeeCap = tx.GasFeeCap()
		transaction.GasTipCap = tx.GasTipCap()
		// if the transaction has been mined, compute the effective gas price
		if baseFee != nil {
			price := types.EffectiveGasPrice(baseFee, tx.GasFeeCap(), tx.GasTipCap())
			transaction.GasPrice = price
		}
	}
	return transaction
}

func BuildPilelineBlockHeader(header map[string]interface{}) *dtypes.Header {
	blockHeader := dtypes.Header{
		Number:           rawHexBig(header["number"]),
		Hash:             rawHash(header["hash"]),
		ParentHash:       header["parentHash"].(common.Hash),
		Nonce:            header["nonce"].(ethtypes.BlockNonce),
		MixHash:          header["mixHash"].(common.Hash),
		Sha3Uncles:       header["sha3Uncles"].(common.Hash),
		LogsBloom:        header["logsBloom"].(ethtypes.Bloom),
		StateRoot:        rawHash(header["stateRoot"]),
		Miner:            header["miner"].(common.Address),
		Difficulty:       rawHexBig(header["difficulty"]),
		ExtraData:        hexutil.Bytes{},
		GasLimit:         hexutil.Uint64(rawUint64(header["gasLimit"])),
		GasUsed:          hexutil.Uint64(rawUint64(header["gasUsed"])),
		Timestamp:        hexutil.Uint64(rawUint64(header["timestamp"])),
		TransactionsRoot: header["transactionsRoot"].(common.Hash),
		ReceiptsRoot:     header["receiptsRoot"].(common.Hash),
	}
	if baseFeePerGas, ok := header["baseFeePerGas"]; ok {
		blockHeader.BaseFeePerGas = rawHexBig(baseFeePerGas)
	}
	return &blockHeader
}

func rawBig(v interface{}) *big.Int {
	switch value := v.(type) {
	case *hexutil.Big:
		if value == nil {
			return big.NewInt(0)
		}
		return new(big.Int).Set((*big.Int)(value))
	case hexutil.Big:
		return new(big.Int).Set((*big.Int)(&value))
	case *big.Int:
		if value == nil {
			return big.NewInt(0)
		}
		return new(big.Int).Set(value)
	case big.Int:
		return new(big.Int).Set(&value)
	case hexutil.Uint64:
		return new(big.Int).SetUint64(uint64(value))
	case uint64:
		return new(big.Int).SetUint64(value)
	case uint:
		return new(big.Int).SetUint64(uint64(value))
	case int64:
		return big.NewInt(value)
	case int:
		return big.NewInt(int64(value))
	default:
		panic(fmt.Sprintf("unsupported numeric field type %T", v))
	}
}

func rawHexBig(v interface{}) *hexutil.Big {
	return (*hexutil.Big)(rawBig(v))
}

func rawUint64(v interface{}) uint64 {
	return rawBig(v).Uint64()
}

func rawHash(v interface{}) common.Hash {
	switch value := v.(type) {
	case common.Hash:
		return value
	case *common.Hash:
		if value == nil {
			return common.Hash{}
		}
		return *value
	case hexutil.Bytes:
		return common.BytesToHash(value)
	case []byte:
		return common.BytesToHash(value)
	default:
		panic(fmt.Sprintf("unsupported hash field type %T", v))
	}
}

func BuildBlockStateDiff(parentRoot common.Hash, root common.Hash, diffs []dtypes.TransactionStateDiff) dtypes.BlockStorageDiff {
	storageDiff := dtypes.BlockStorageDiff{
		Hash:            root,
		ParentHash:      parentRoot,
		NewAccounts:     make([]dtypes.NewAccount, 0),
		NewCodes:        make([]dtypes.NewCode, 0),
		DeletedAccounts: make([]common.Hash, 0),
		StorageDiff:     make([]dtypes.AccountStorageDiff, 0),
	}
	newAccountMap := make(map[common.Hash]dtypes.NewAccount)
	deleteAccountMap := make(map[common.Hash]struct{})
	codeMap := make(map[common.Hash]dtypes.NewCode)

	mergedStorage := make(map[common.Hash]map[common.Hash]*uint256.Int)

	for _, diff := range diffs {
		for _, deletedAccount := range diff.DeletedAccounts {
			if newAccount, ok := newAccountMap[deletedAccount]; ok {
				delete(codeMap, newAccount.CodeHash)
				delete(newAccountMap, deletedAccount)
				delete(mergedStorage, deletedAccount)
				continue
			}

			delete(newAccountMap, deletedAccount)
			delete(mergedStorage, deletedAccount)
			deleteAccountMap[deletedAccount] = struct{}{}
		}

		for _, newCode := range diff.NewCodes {
			codeMap[newCode.CodeHash] = newCode
		}
		for _, newAccount := range diff.NewAccounts {
			newAccountMap[newAccount.Address] = newAccount
			delete(deleteAccountMap, newAccount.Address)
		}
		for _, accountStorageDiff := range diff.StorageDiff {
			addr := accountStorageDiff.Address
			if mergedStorage[addr] == nil {
				mergedStorage[addr] = make(map[common.Hash]*uint256.Int)
			}
			for _, kv := range accountStorageDiff.Values {
				mergedStorage[addr][kv.Index] = kv.Value
			}
		}
	}

	for deleteAccount := range deleteAccountMap {
		storageDiff.DeletedAccounts = append(storageDiff.DeletedAccounts, deleteAccount)
	}
	for _, account := range newAccountMap {
		storageDiff.NewAccounts = append(storageDiff.NewAccounts, account)
	}
	for _, code := range codeMap {
		storageDiff.NewCodes = append(storageDiff.NewCodes, code)
	}

	for addr, slots := range mergedStorage {
		accountDiff := dtypes.AccountStorageDiff{
			Address: addr,
			Values:  make([]dtypes.IndexValuePair, 0, len(slots)),
		}
		for index, value := range slots {
			accountDiff.Values = append(accountDiff.Values, dtypes.IndexValuePair{
				Index: index,
				Value: value,
			})
		}

		sort.Slice(accountDiff.Values, func(i, j int) bool {
			return accountDiff.Values[i].Index.Hex() < accountDiff.Values[j].Index.Hex()
		})

		storageDiff.StorageDiff = append(storageDiff.StorageDiff, accountDiff)
	}

	return storageDiff
}
