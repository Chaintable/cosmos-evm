package trace

type Tx struct {
	Body struct {
		Messages []struct {
			Type        string `json:"@type"`
			Description struct {
				Moniker         string `json:"moniker"`
				Identity        string `json:"identity"`
				Website         string `json:"website"`
				SecurityContact string `json:"security_contact"`
				Details         string `json:"details"`
			} `json:"description"`
			Commission struct {
				Rate          string `json:"rate"`
				MaxRate       string `json:"max_rate"`
				MaxChangeRate string `json:"max_change_rate"`
			} `json:"commission"`
			MinSelfDelegation string `json:"min_self_delegation"`
			DelegatorAddress  string `json:"delegator_address"`
			ValidatorAddress  string `json:"validator_address"`
			Pubkey            struct {
				Type string `json:"@type"`
				Key  string `json:"key"`
			} `json:"pubkey"`
			Value struct {
				Denom  string `json:"denom"`
				Amount string `json:"amount"`
			} `json:"value"`
		} `json:"messages"`
		Memo                        string        `json:"memo"`
		TimeoutHeight               string        `json:"timeout_height"`
		ExtensionOptions            []interface{} `json:"extension_options"`
		NonCriticalExtensionOptions []interface{} `json:"non_critical_extension_options"`
	} `json:"body"`
	AuthInfo struct {
		SignerInfos []struct {
			PublicKey struct {
				Type string `json:"@type"`
				Key  string `json:"key"`
			} `json:"public_key"`
			ModeInfo struct {
				Single struct {
					Mode string `json:"mode"`
				} `json:"single"`
			} `json:"mode_info"`
			Sequence string `json:"sequence"`
		} `json:"signer_infos"`
		Fee struct {
			Amount []struct {
				Denom  string `json:"denom"`
				Amount string `json:"amount"`
			} `json:"amount"`
			GasLimit string `json:"gas_limit"`
			Payer    string `json:"payer"`
			Granter  string `json:"granter"`
		} `json:"fee"`
		Tip interface{} `json:"tip"`
	} `json:"auth_info"`
	Signatures []string `json:"signatures"`
}
