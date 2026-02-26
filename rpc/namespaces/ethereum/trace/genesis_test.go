package trace

import (
	"encoding/json"
	"testing"
)

func TestAppState(t *testing.T) {
	state, err := appState()
	if err != nil {
		t.Fatal(err)
	}
	marshal, err := json.Marshal(state)
	if err != nil {
		return
	}
	t.Log(string(marshal))
}

func TestStates(t *testing.T) {
	state, err := appState()
	if err != nil {
		t.Error(err)
	}
	{
		evm, err := evmState(state)
		if err != nil {
			t.Error(err)
		}
		t.Log(evm)
	}
	{
		bank, err := bankState(state)
		if err != nil {
			t.Error(err)
		}
		t.Log(bank)
	}
	{
		gen, err := genutilState(state)
		if err != nil {
			t.Error(err)
		}
		t.Log(gen)
		txs, err := collectTxs(gen)
		if err != nil {
			t.Error(err)
			return
		}
		for _, tx := range txs {
			t.Log(tx.Body.Messages[0].ValidatorAddress)
		}
	}
}

func TestBech32(t *testing.T) {
	state, err := appState()
	if err != nil {
		t.Error(err)
	}
	bank, err := bankState(state)
	if err != nil {
		t.Error(err)
	}
	for _, balance := range bank.Balances {
		addr, err := bech32ToAddress(balance.Address)
		if err != nil {
			t.Error(err)
		}
		t.Log(addr)
	}
}
