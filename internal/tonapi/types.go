package tonapi

// Event represents a TonAPI event
type Event struct {
	EventID   string   `json:"event_id"`
	Timestamp int64    `json:"timestamp"`
	Actions   []Action `json:"actions"`
	IsScam    bool     `json:"is_scam"`
}

// Action represents an action within an event
type Action struct {
	Type        string       `json:"type"`
	Status      string       `json:"status"`
	TonTransfer *TonTransfer `json:"TonTransfer,omitempty"`
	JettonSwap  *JettonSwap  `json:"JettonSwap,omitempty"`
}

// TonTransfer represents a TON transfer action
type TonTransfer struct {
	Sender    Account `json:"sender"`
	Recipient Account `json:"recipient"`
	Amount    int64   `json:"amount"` // in nanoTON
	Comment   string  `json:"comment,omitempty"`
}

// JettonSwap represents a DEX swap action
type JettonSwap struct {
	Dex             string       `json:"dex"`
	TonIn           int64        `json:"ton_in,omitempty"`
	TonOut          int64        `json:"ton_out,omitempty"`
	AmountIn        string       `json:"amount_in,omitempty"`
	AmountOut       string       `json:"amount_out,omitempty"`
	JettonMasterIn  *JettonInfo  `json:"jetton_master_in,omitempty"`
	JettonMasterOut *JettonInfo  `json:"jetton_master_out,omitempty"`
	Router          Account      `json:"router"`
}

// JettonInfo contains jetton metadata
type JettonInfo struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	Image    string `json:"image,omitempty"`
}

// Account represents an account/wallet
type Account struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	IsScam   bool   `json:"is_scam,omitempty"`
	IsWallet bool   `json:"is_wallet,omitempty"`
}

// AccountInfo contains account information
type AccountInfo struct {
	Address string `json:"address"` // raw format
	Balance int64  `json:"balance"`
	Status  string `json:"status"`
}

// EventsResponse is the response from events endpoint
type EventsResponse struct {
	Events []Event `json:"events"`
}

// WebhookPayload is the payload received from TonAPI webhook
type WebhookPayload struct {
	EventType string `json:"event_type,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	TxHash    string `json:"tx_hash,omitempty"`
	Lt        int64  `json:"lt,omitempty"`
	Event     *Event `json:"event,omitempty"`
}

// Webhook represents a TonAPI webhook
type Webhook struct {
	ID        int64    `json:"webhook_id"`
	Endpoint  string   `json:"endpoint"`
	Accounts  []string `json:"subscribed_accounts,omitempty"`
}

// WebhookListResponse is the response from webhook list endpoint
type WebhookListResponse struct {
	Webhooks []Webhook `json:"webhooks"`
}
