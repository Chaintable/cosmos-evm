package debank

import (
	"encoding/json"
	"math/big"

	rpctypes "github.com/cosmos/evm/rpc/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CallArgs struct {
	From     *common.Address `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Data     *hexutil.Bytes  `json:"data"`
	Nonce    *hexutil.Uint64 `json:"nonce"`
	ChainID  *big.Int        `json:"chainId,omitempty"`
}

func (a *API) SimulateTransactions(args []CallArgs, blockContext *rpctypes.DebankBlockContext, _ *rpctypes.BlockOverrides) (*rpctypes.DebankSimulateResp, error) {
	latestBlockNumber := rpctypes.EthLatestBlockNumber
	blockNrOrHash := rpctypes.BlockNumberOrHash{BlockNumber: &latestBlockNumber}
	if blockContext != nil {
		blockNrOrHash = blockContext.GetBlockNumberOrHash()
	}
	a.logger.Debug("simulateTransactions", "args", args)

	rpcArgs := evmtypes.TransactionArgs{}
	for _, arg := range args {
		realArgs := evmtypes.TransactionArgs{
			From:     arg.From,
			To:       arg.To,
			Gas:      arg.Gas,
			GasPrice: arg.GasPrice,
			Value:    arg.Value,
			Data:     arg.Data,
			Nonce:    arg.Nonce,
		}
		rpcArgs.Args = append(rpcArgs.Args, realArgs)
	}
	blockNum, err := a.backend.BlockNumberFromTendermint(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	resBlock, err := a.backend.TendermintBlockByNumber(blockNum)
	if err != nil {
		a.logger.Debug("get block failed", "height", blockNum, "error", err.Error())
		return nil, err
	}
	blockHash := common.BytesToHash(resBlock.BlockID.Hash)
	rpcArgs.BlockHash = &blockHash

	bz, err := json.Marshal(&rpcArgs)
	if err != nil {
		a.logger.Error("json.Marshal faield", "args", args, "err", err)
		return nil, err
	}
	req := evmtypes.EthCallRequest{
		Args:   bz,
		GasCap: a.backend.RPCGasCap(),
	}

	// From ContextWithHeight: if the provided height is 0,
	// it will return an empty context and the gRPC query will use
	// the latest block height for querying.
	ctx := rpctypes.ContextWithHeight(int64(blockNum))
	res, err := a.queryClient.EthCall(ctx, &req)
	if err != nil {
		a.logger.Error("EthCall faield", "req", req, "err", err)
		return nil, err
	}
	var simulateRes []evmtypes.DebankSingleSimulateResult
	err = json.Unmarshal(res.Ret, &simulateRes)
	if err != nil {
		a.logger.Error("json.Unmarshal faield", "err", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return applyBlockContext(simulateRes), nil
}

func applyBlockContext(simulateResList []evmtypes.DebankSingleSimulateResult) *rpctypes.DebankSimulateResp {
	return nil
}
