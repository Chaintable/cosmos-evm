package tracer

import (
	"testing"

	dtypes "github.com/cosmos/evm/debank/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func addAccount(diff *dtypes.TransactionStateDiff, address common.Hash, codeHash common.Hash) {
	diff.NewAccounts = append(diff.NewAccounts, dtypes.NewAccount{
		Address:  address,
		Balance:  uint256.NewInt(0),
		Nonce:    0,
		CodeHash: codeHash,
	})
	diff.NewCodes = append(diff.NewCodes, dtypes.NewCode{CodeHash: codeHash})
}

func deleteAccount(diff *dtypes.TransactionStateDiff, address common.Hash) {
	diff.DeletedAccounts = append(diff.DeletedAccounts, address)
}

func TestBuildBlockStateDiff(t *testing.T) {
	var (
		hash1 = common.HexToHash("0x8deab29d3e78138cb6398713f1d04a457e28882d5c08b0cb7923c72360747033")
		hash2 = common.HexToHash("0x2c405b6fedb744133e8d1952bfb6cc639aadd067c6f06a7d4b6e255e38d674f9")
		hash3 = common.HexToHash("0x11c2987f2265e06910f4a272c1690dcd5ac991b36351f81480cce37c4b777dc2")
		hash4 = common.HexToHash("0x9e120e3533d2a035e0e32d8ea470c8b2ab2e00659d6ed90d6350c617b991174d")

		codeHash1 = common.HexToHash("0xaec34a38a6456d2c1fc0d2e41923c4ed6586b4d9e84c589fc712350b1c9e8244")
		codeHash2 = common.HexToHash("0xfe97e1286a07a0412615a4cdcd4f161a37f0112f4b37f9a298157ae1cb740e69")
		codeHash3 = common.HexToHash("0xbf18b2b495d6f4ad423c7fd2958c19382079cb8b4ce101aae23d80f03f31cc96")
		//codeHash4 = common.HexToHash("0xab2670f36c2b214e6cbd2c6c02f4e4bda4ff0568a5eb5235f050e721be679193")
	)

	txDiffs := make([]dtypes.TransactionStateDiff, 0)

	txDiff1 := new(dtypes.TransactionStateDiff)
	addAccount(txDiff1, hash1, codeHash1)
	deleteAccount(txDiff1, hash2)
	txDiffs = append(txDiffs, *txDiff1)

	txDiff2 := new(dtypes.TransactionStateDiff)
	addAccount(txDiff2, hash2, codeHash2)
	deleteAccount(txDiff2, hash1)
	txDiffs = append(txDiffs, *txDiff2)

	blockStateDiff := BuildBlockStateDiff(types.EmptyRootHash, types.EmptyRootHash, txDiffs)
	require.Len(t, blockStateDiff.NewAccounts, 1)
	require.Empty(t, blockStateDiff.DeletedAccounts)
	require.Equal(t, hash2, blockStateDiff.NewAccounts[0].Address)
	require.Len(t, blockStateDiff.NewCodes, 1)
	require.Equal(t, codeHash2, blockStateDiff.NewCodes[0].CodeHash)

	txDiff3 := new(dtypes.TransactionStateDiff)
	addAccount(txDiff3, hash3, codeHash3)
	deleteAccount(txDiff3, hash4)
	deleteAccount(txDiff3, hash2)
	txDiffs = append(txDiffs, *txDiff3)

	blockStateDiff = BuildBlockStateDiff(types.EmptyRootHash, types.EmptyRootHash, txDiffs)
	require.Len(t, blockStateDiff.NewAccounts, 1)
	require.Len(t, blockStateDiff.DeletedAccounts, 1)
	require.Equal(t, hash3, blockStateDiff.NewAccounts[0].Address)
	require.Equal(t, hash4, blockStateDiff.DeletedAccounts[0])
	require.Len(t, blockStateDiff.NewCodes, 1)
	require.Equal(t, codeHash3, blockStateDiff.NewCodes[0].CodeHash)
}
