package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDebankContest(t *testing.T) {
	input := "{\"block_id\":\"latest\",\"type\":\"Contains\"}"
	var context *DebankBlockContext
	var blockNumber = EthLatestBlockNumber
	err := json.Unmarshal([]byte(input), &context)
	require.NoError(t, err)
	require.Equal(t, &DebankBlockContext{
		BlockId: BlockNumberOrHash{
			BlockNumber: &blockNumber,
		},
		BlockType: BlockTypeContains,
	}, context)
}
