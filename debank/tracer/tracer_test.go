package tracer

import (
	"strings"
	"testing"

	dtypes "github.com/cosmos/evm/debank/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/stretchr/testify/require"
)

func testAddress(value byte) common.Address {
	return common.Address{19: value}
}

func testCallFrame(value byte, typ vm.OpCode, err string, calls ...callFrame) callFrame {
	to := testAddress(value)
	return callFrame{
		Type:  typ,
		To:    &to,
		Error: err,
		Calls: calls,
	}
}

func processCallTree(root callFrame) *CallTracer {
	tracer := NewCallTracer(&tracers.Context{TxHash: common.HexToHash("0x1234")})
	tracer.callstack = []callFrame{root}
	tracer.OnTxEnd(nil, nil)
	return tracer
}

func requireTraceForAddress(t *testing.T, traces []dtypes.Trace, address common.Address) dtypes.Trace {
	t.Helper()
	expected := strings.ToLower(address.Hex())
	for _, trace := range traces {
		if trace.To == expected {
			return trace
		}
	}
	require.FailNow(t, "trace not found", "address: %s", expected)
	return dtypes.Trace{}
}

func TestSuccessfulDescendantsOfFailedCallAreErrorTraces(t *testing.T) {
	root := testCallFrame(
		1,
		vm.CALL,
		"",
		testCallFrame(
			2,
			vm.CALL,
			"execution reverted",
			testCallFrame(3, vm.CALL, ""),
			testCallFrame(4, vm.CALL, "out of gas"),
		),
		testCallFrame(5, vm.CALL, ""),
	)

	tracer := processCallTree(root)
	traces := tracer.GetTraces()
	errorTraces := tracer.GetErrorTraces()

	require.Len(t, traces, 2)
	require.Empty(t, requireTraceForAddress(t, traces, testAddress(1)).Error)
	require.Empty(t, requireTraceForAddress(t, traces, testAddress(5)).Error)

	require.Len(t, errorTraces, 3)
	require.Equal(t, "execution reverted", requireTraceForAddress(t, errorTraces, testAddress(2)).Error)
	require.Equal(t, parentCallFailedError, requireTraceForAddress(t, errorTraces, testAddress(3)).Error)
	require.Equal(t, "out of gas", requireTraceForAddress(t, errorTraces, testAddress(4)).Error)
}

func TestFailedTopLevelCallMarksSuccessfulDescendants(t *testing.T) {
	root := testCallFrame(
		1,
		vm.CALL,
		"execution reverted",
		testCallFrame(2, vm.CALL, "", testCallFrame(3, vm.CALL, "")),
	)

	tracer := processCallTree(root)
	errorTraces := tracer.GetErrorTraces()

	require.Empty(t, tracer.GetTraces())
	require.Len(t, errorTraces, 3)
	require.Equal(t, "execution reverted", requireTraceForAddress(t, errorTraces, testAddress(1)).Error)
	require.Equal(t, parentCallFailedError, requireTraceForAddress(t, errorTraces, testAddress(2)).Error)
	require.Equal(t, parentCallFailedError, requireTraceForAddress(t, errorTraces, testAddress(3)).Error)
}

func TestSelfDestructUnderFailedParentIsErrorTrace(t *testing.T) {
	root := testCallFrame(
		1,
		vm.CALL,
		"execution reverted",
		testCallFrame(2, vm.SELFDESTRUCT, ""),
	)

	tracer := processCallTree(root)
	errorTraces := tracer.GetErrorTraces()

	require.Empty(t, tracer.GetTraces())
	require.Len(t, errorTraces, 2)
	selfDestructTrace := requireTraceForAddress(t, errorTraces, testAddress(2))
	require.Equal(t, "suicide", selfDestructTrace.CallCreateType)
	require.Equal(t, parentCallFailedError, selfDestructTrace.Error)
}

func TestSuccessfulCallTreeHasNoErrorTraces(t *testing.T) {
	root := testCallFrame(
		1,
		vm.CALL,
		"",
		testCallFrame(2, vm.CALL, "", testCallFrame(3, vm.CALL, "")),
	)

	tracer := processCallTree(root)
	traces := tracer.GetTraces()

	require.Len(t, traces, 3)
	for _, trace := range traces {
		require.Empty(t, trace.Error)
	}
	require.Empty(t, tracer.GetErrorTraces())
}
