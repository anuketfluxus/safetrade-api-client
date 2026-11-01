package safetrade

import (
	"github.com/shopspring/decimal"
)

type Balance struct {
	Currency string          `json:"currency"`
	Type     AccountType     `json:"type"`
	Balance  decimal.Decimal `json:"balance"`

	Locked           decimal.Decimal  `json:"locked"`
	DepositAddresses []DepositAddress `json:"deposit_addresses"`
}

type DepositAddress struct {
	Address       string `json:"address"`
	Network       string `json:"network"`
	ParentAddress string `json:"parent_address"`

	Currencies []string `json:"currencies"`
}

type Beneficiary struct {
	ID            int64            `json:"id"`
	CurrencyID    string           `json:"currency_id"`
	BlockchainKey string           `json:"blockchain_key"`
	Address       string           `json:"address"`
	Label         string           `json:"label"`
	Description   string           `json:"description"`
	State         BeneficiaryState `json:"state"`

	Options   map[string]any `json:"options"`
	CreatedAt Timestamp      `json:"created_at"`
	UpdatedAt Timestamp      `json:"updated_at"`
}

type Deposit struct {
	ID            int64           `json:"id"`
	Currency      string          `json:"currency"`
	BlockchainKey string          `json:"blockchain_key"`
	Amount        decimal.Decimal `json:"amount"`
	Fee           decimal.Decimal `json:"fee"`
	FeeCurrency   string          `json:"fee_currency"`
	FeePaid       bool            `json:"fee_paid"`

	Credited    bool          `json:"credited"`
	FromAddress string        `json:"from_address"`
	TxID        string        `json:"txid"`
	Status      DepositStatus `json:"status"`

	Options   map[string]any `json:"options"`
	CreatedAt Timestamp      `json:"created_at"`
	UpdatedAt Timestamp      `json:"updated_at"`

	CompletedAt Timestamp `json:"completed_at"`
}

type Member struct {
	ID       int64  `json:"id"`
	UID      string `json:"uid"`
	Email    string `json:"email"`
	Username string `json:"username"`

	Level int    `json:"level"`
	Role  string `json:"role"`

	Group string `json:"group"`
	State string `json:"state"`

	WithdrawPasswordEnabled bool      `json:"withdraw_password_enabled"`
	CreatedAt               Timestamp `json:"created_at"`
	UpdatedAt               Timestamp `json:"updated_at"`
}

type Withdrawal struct {
	ID            int64            `json:"id"`
	Type          WithdrawalType   `json:"type"`
	CurrencyID    string           `json:"currency_id"`
	BlockchainKey string           `json:"blockchain_key"`
	BeneficiaryID int64            `json:"beneficiary_id"`
	RID           string           `json:"rid"`
	TID           string           `json:"tid"`
	Amount        decimal.Decimal  `json:"amount"`
	Fee           decimal.Decimal  `json:"fee"`
	FeeCurrencyID string           `json:"fee_currency_id"`
	Sum           decimal.Decimal  `json:"sum"`
	Total         decimal.Decimal  `json:"total"`
	TxID          string           `json:"txid"`
	BlockNumber   int64            `json:"block_number"`
	Status        WithdrawalStatus `json:"status"`

	Options   map[string]any `json:"options"`
	CreatedAt Timestamp      `json:"created_at"`
	UpdatedAt Timestamp      `json:"updated_at"`

	CompletedAt Timestamp `json:"completed_at"`
}
