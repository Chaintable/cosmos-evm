package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"

	"cosmossdk.io/log"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/evm/rpc/backend"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"

	dtracer "github.com/cosmos/evm/debank/tracer"
	dtypes "github.com/cosmos/evm/debank/types"
	rpctypes "github.com/cosmos/evm/rpc/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

var (
	LatestBlockNumber = metrics.NewRegisteredGauge("pipeline/block_num", nil)

	LatestBlockTime = metrics.NewRegisteredGauge("pipeline/block_time", nil)

	NodeInfo = metrics.NewRegisteredGaugeInfo("pipeline/node_info", nil)
)

// HandlerT keeps track of the cpu profiler and trace execution
type HandlerT struct {
	cpuFilename   string
	cpuFile       io.WriteCloser
	mu            sync.Mutex
	traceFilename string
	traceFile     io.WriteCloser
}

// API is the collection of tracing APIs exposed over the private debugging endpoint.
type API struct {
	ctx         *server.Context
	logger      log.Logger
	backend     *backend.Backend
	clientCtx   client.Context
	queryClient *rpctypes.QueryClient
	handler     *HandlerT
}

// NewAPI creates a new API definition for the tracing methods of the Ethereum service.
func NewAPI(
	ctx *server.Context,
	logger log.Logger,
	backend *backend.Backend,
	clientCtx client.Context,
) *API {
	NodeInfo.Update(map[string]string{
		"chain_id": backend.ChainConfig().ChainID.String(),
		"role":     "writer",
	})
	return &API{
		ctx:         ctx,
		logger:      logger.With("module", "trace"),
		backend:     backend,
		clientCtx:   clientCtx,
		queryClient: rpctypes.NewQueryClient(clientCtx),
		handler:     new(HandlerT),
	}
}

func (api *API) DebankBlockRaw(ctx context.Context, blockNrOrHash rpctypes.BlockNumberOrHash) (*dtypes.DebankOutPut, error) {
	blockHeight, err := api.backend.BlockNumberFromTendermint(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	if blockHeight == 0 {
		return nil, fmt.Errorf("xrplevm can't trace block 0")
	}

	resBlock, err := api.backend.TendermintBlockByNumber(blockHeight)
	if err != nil {
		return nil, nil
	}

	// return if requested block height is greater than the current one
	if resBlock == nil || resBlock.Block == nil {
		return nil, fmt.Errorf("cannot trace nil block")
	}

	blockRes, err := api.backend.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		api.logger.Debug("failed to fetch block result from Tendermint", "height", blockHeight, "error", err.Error())
		return nil, fmt.Errorf("failed to fetch block result from Tendermint")
	}
	block, err := api.backend.RPCBlockFromTendermintBlock(resBlock, blockRes, true)
	if err != nil {
		api.logger.Debug("GetEthBlockFromTendermint failed", "height", blockHeight, "error", err.Error())
		return nil, err
	}
	transactions := block["transactions"].([]interface{})
	stateHeader := dtracer.BuildPilelineBlockHeader(block)
	parentHeader, err := api.backend.HeaderByNumber(blockHeight - 1)
	if err != nil {
		return nil, err
	}
	blockFile := &dtypes.BlockFile{
		Block:            dtracer.BuildPipelineBlock(block),
		Events:           make([]dtypes.Event, 0),
		Txs:              make([]dtypes.Transaction, 0),
		Traces:           make([]dtypes.Trace, 0),
		ErrorEvents:      make([]dtypes.Event, 0),
		ErrorTraces:      make([]dtypes.Trace, 0),
		StorageContracts: make([]string, 0),
	}
	traceResults, err := api.backend.TraceBlock(blockHeight, &evmtypes.TraceConfig{Tracer: dtracer.Name}, resBlock)
	if err != nil {
		return nil, err
	}
	transactionStates := make([]dtypes.TransactionStateDiff, 0)
	transactionHash := make(map[common.Hash]bool)
	transactionFromAddress := make(map[common.Address]struct{})
	for i := range transactions {
		transaction := transactions[i].(*rpctypes.RPCTransaction)
		transactionHash[transaction.Hash] = true
		transactionFromAddress[transaction.From] = struct{}{}
	}
	for i, result := range traceResults {
		traceResultRaw, ok := result.Result.(map[string]interface{})
		if !ok {
			return nil, status.Error(codes.Internal, "trace result parse error")
		}
		decoded, err := json.Marshal(traceResultRaw)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		var traceResult dtypes.TraceResult
		if err = json.Unmarshal(decoded, &traceResult); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("trace result parse error: %v", err))
		}
		// rpc返回的结果不包含失败的transaction，但是trace的会包含，所以需要过滤
		if !transactionHash[common.HexToHash(traceResult.Transaction.ID)] {
			continue
		}
		traceResult.Transaction.ID = transactions[i].(*rpctypes.RPCTransaction).Hash.Hex()
		traceResult.Transaction.GasPrice = (*big.Int)(transactions[i].(*rpctypes.RPCTransaction).GasPrice)
		blockFile.Txs = append(blockFile.Txs, traceResult.Transaction)
		blockFile.Traces = append(blockFile.Traces, traceResult.Traces...)
		blockFile.Events = append(blockFile.Events, traceResult.Events...)
		blockFile.ErrorEvents = append(blockFile.ErrorEvents, traceResult.ErrorEvents...)
		blockFile.ErrorTraces = append(blockFile.ErrorTraces, traceResult.ErrorTraces...)
		blockFile.StorageContracts = append(blockFile.StorageContracts, traceResult.StorageContracts...)
		transactionStates = append(transactionStates, traceResult.StateDiff)
	}
	for i := range blockFile.Events {
		blockFile.Events[i].LogIndex = int64(i)
	}
	var parentRoot = parentHeader.Root
	if blockHeight == 1 {
		parentRoot = ethtypes.EmptyRootHash
	}
	stateDiff := dtracer.BuildBlockStateDiff(parentRoot, stateHeader.StateRoot, transactionStates)
	// 通过tracer获得的stateDiff拿不到tx的gasUsed的变化，进行后处理
	newAccounts, storageContracts, err := api.addGasUsedStateDiff(transactionFromAddress, stateDiff.NewAccounts, blockFile.StorageContracts, blockHeight)
	if err != nil {
		return nil, err
	}
	stateDiff.NewAccounts = newAccounts
	blockFile.StorageContracts = storageContracts
	out := &dtypes.DebankOutPut{
		BlockFile:      blockFile,
		Header:         stateHeader,
		StateDiff:      &stateDiff,
		ValidationHash: blockFile.Validation().ValidationHash,
	}
	return out, nil
}

func (api API) DebankBlock(ctx context.Context, blockNrOrHash rpctypes.BlockNumberOrHash) (*rpctypes.DebankOutPutJs, error) {
	output, err := api.DebankBlockRaw(ctx, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	data, err := rlp.EncodeToBytes(output.StateDiff)
	if err != nil {
		return nil, err
	}
	LatestBlockNumber.Update(int64(output.Header.Number.ToInt().Uint64()))
	LatestBlockTime.Update(int64(output.Header.Timestamp))

	return &rpctypes.DebankOutPutJs{
		BlockFile:      output.BlockFile,
		Header:         output.Header,
		StateDiff:      data,
		ValidationHash: output.ValidationHash,
	}, nil
}

func (api API) addGasUsedStateDiff(txFromAddress map[common.Address]struct{}, newAccount []dtypes.NewAccount, storageChange []string, number rpctypes.BlockNumber) ([]dtypes.NewAccount, []string, error) {
	var (
		newAccountMap    = make(map[common.Hash]dtypes.NewAccount)
		storageChangeMap = make(map[common.Address]struct{})
	)
	for _, account := range newAccount {
		newAccountMap[account.Address] = account
	}
	for _, address := range storageChange {
		storageChangeMap[common.HexToAddress(address)] = struct{}{}
	}
	for addr := range txFromAddress {
		var addrHash = crypto.Keccak256Hash(addr.Bytes())
		balance, err := api.backend.GetBalance(addr, rpctypes.BlockNumberOrHash{BlockNumber: &number})
		if err != nil {
			return nil, nil, err
		}
		api.logger.Info("get balance", "address", addr.String(), "balance", balance.String(), "block number", number.Int64())
		nonce, err := api.backend.GetTransactionCount(addr, number)
		if err != nil {
			return nil, nil, err
		}
		code, err := api.backend.GetCode(addr, rpctypes.BlockNumberOrHash{BlockNumber: &number})
		if err != nil {
			return nil, nil, err
		}
		newAccountMap[addrHash] = dtypes.NewAccount{
			Address:  addrHash,
			Balance:  uint256.MustFromBig((*big.Int)(balance)),
			Nonce:    uint64(*nonce),
			CodeHash: crypto.Keccak256Hash(code),
		}
		storageChangeMap[addr] = struct{}{}
	}

	var (
		resNewAccount    = make([]dtypes.NewAccount, 0, len(newAccountMap))
		resStorageChange = make([]string, 0, len(storageChangeMap))
	)
	for _, acc := range newAccountMap {
		resNewAccount = append(resNewAccount, acc)
	}
	for address := range storageChangeMap {
		resStorageChange = append(resStorageChange, strings.ToLower(address.String()))
	}
	return resNewAccount, resStorageChange, nil
}
