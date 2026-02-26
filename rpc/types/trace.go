package types

import (
	dtypes "github.com/cosmos/evm/debank/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type DebankOutPutJs struct {
	BlockFile      *dtypes.BlockFile `json:"block_file"`
	Header         *dtypes.Header    `json:"header"`
	StateDiff      hexutil.Bytes     `json:"state_diff"`
	ValidationHash int64             `json:"validation_hash"`
}
