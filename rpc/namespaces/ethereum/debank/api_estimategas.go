package debank

import (
	rpctypes "github.com/cosmos/evm/rpc/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func (a *API) EstimateGas(args evmtypes.TransactionArgs, blockContext *rpctypes.DebankBlockContext, _ *rpctypes.BlockOverrides) (*hexutil.Uint64, error) {
	a.logger.Debug("debank_estimateGas")
	latestBlockNumber := rpctypes.EthLatestBlockNumber
	blockNrOrHash := rpctypes.BlockNumberOrHash{BlockNumber: &latestBlockNumber}
	if blockContext != nil {
		blockNrOrHash = blockContext.GetBlockNumberOrHash()
	}
	blockNum, err := a.backend.BlockNumberFromComet(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	gas, err := a.backend.EstimateGas(args, &blockNum)
	if err != nil {
		return nil, err
	}
	return &gas, nil
}
