package safetrade

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type AccountService struct {
	client *Client
}

func (s *AccountService) GetMember(ctx context.Context) (Member, error) {
	return call[Member](ctx, s.client, privateGet("/trade/account/members/me", nil))
}

func (s *AccountService) ListSpotBalances(ctx context.Context) ([]Balance, error) {
	return call[[]Balance](ctx, s.client, privateGet("/trade/account/balances/spot", nil))
}

type ListBeneficiariesParams struct {
	State         BeneficiaryState
	BlockchainKey string
	CurrencyID    string

	Page  int
	Limit int
}

func (s *AccountService) ListBeneficiaries(ctx context.Context, params ListBeneficiariesParams) ([]Beneficiary, error) {
	q := query{}
	q.set("state", string(params.State))
	q.set("blockchain_key", params.BlockchainKey)
	q.set("currency_id", params.CurrencyID)
	q.setPage(params.Page, params.Limit)
	return call[[]Beneficiary](ctx, s.client, privateGet("/trade/account/beneficiaries", q))
}

func (s *AccountService) GetDepositAddress(ctx context.Context, currencyID, network string) (DepositAddress, error) {
	segment, err := pathSegment("currency ID", currencyID)
	if err != nil {
		return DepositAddress{}, err
	}
	if network == "" {
		return DepositAddress{}, invalidArgument("network must not be empty")
	}
	q := query{}
	q.set("network", network)
	return call[DepositAddress](ctx, s.client, privateGet("/trade/account/deposit_address/"+segment, q))
}

type ListDepositsParams struct {
	Currency      string
	Status        DepositStatus
	BlockchainKey string
	TimeFrom      time.Time
	TimeTo        time.Time
	OrderBy       string
	Ordering      Ordering

	Page  int
	Limit int
}

func (s *AccountService) ListDeposits(ctx context.Context, params ListDepositsParams) ([]Deposit, error) {
	q := query{}
	q.set("currency", params.Currency)
	q.set("status", string(params.Status))
	q.set("blockchain_key", params.BlockchainKey)
	q.setUnix("time_from", params.TimeFrom)
	q.setUnix("time_to", params.TimeTo)
	q.setSort(params.OrderBy, params.Ordering)
	q.setPage(params.Page, params.Limit)
	return call[[]Deposit](ctx, s.client, privateGet("/trade/account/deposits", q))
}

func (s *AccountService) GetDeposit(ctx context.Context, txid string) (Deposit, error) {
	segment, err := pathSegment("transaction ID", txid)
	if err != nil {
		return Deposit{}, err
	}
	return call[Deposit](ctx, s.client, privateGet("/trade/account/deposits/"+segment, nil))
}

type ListWithdrawalsParams struct {
	Currency      string
	Type          WithdrawalType
	Status        WithdrawalStatus
	BlockchainKey string
	RID           string
	TimeFrom      time.Time
	TimeTo        time.Time
	OrderBy       string
	Ordering      Ordering

	Page  int
	Limit int
}

func (s *AccountService) ListWithdrawals(ctx context.Context, params ListWithdrawalsParams) ([]Withdrawal, error) {
	q := query{}
	q.set("currency", params.Currency)
	q.set("type", string(params.Type))
	q.set("status", string(params.Status))
	q.set("blockchain_key", params.BlockchainKey)
	q.set("rid", params.RID)
	q.setUnix("time_from", params.TimeFrom)
	q.setUnix("time_to", params.TimeTo)
	q.setSort(params.OrderBy, params.Ordering)
	q.setPage(params.Page, params.Limit)
	return call[[]Withdrawal](ctx, s.client, privateGet("/trade/account/withdraws", q))
}

type CreateWithdrawalParams struct {
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	BlockchainKey string          `json:"blockchain_key,omitempty"`

	BeneficiaryID int64 `json:"beneficiary_id,omitempty"`

	Address string `json:"address,omitempty"`

	EmailCode string `json:"email_code,omitempty"`
	PhoneCode string `json:"phone_code,omitempty"`

	OTPCode string `json:"otp_code,omitempty"`

	WithdrawPassword string `json:"withdraw_password,omitempty"`

	Options map[string]any `json:"options,omitempty"`
}

func (s *AccountService) CreateWithdrawal(ctx context.Context, params CreateWithdrawalParams) (Withdrawal, error) {
	if params.Currency == "" {
		return Withdrawal{}, invalidArgument("currency must not be empty")
	}
	if !params.Amount.IsPositive() {
		return Withdrawal{}, invalidArgument("amount must be positive, got %s", params.Amount)
	}
	return call[Withdrawal](ctx, s.client, privatePost("/trade/account/withdraws", params))
}

type GenerateWithdrawalCodeParams struct {
	Currency      string          `json:"currency"`
	BlockchainKey string          `json:"blockchain_key"`
	Address       string          `json:"address"`
	Amount        decimal.Decimal `json:"amount"`

	Channel CodeChannel `json:"type"`
}

func (s *AccountService) GenerateWithdrawalCode(ctx context.Context, params GenerateWithdrawalCodeParams) error {
	switch {
	case params.Currency == "":
		return invalidArgument("currency must not be empty")
	case params.BlockchainKey == "":
		return invalidArgument("blockchain key must not be empty")
	case params.Address == "":
		return invalidArgument("address must not be empty")
	case !params.Amount.IsPositive():
		return invalidArgument("amount must be positive, got %s", params.Amount)
	case params.Channel == "":
		return invalidArgument("channel must not be empty")
	}
	return s.client.do(ctx, privatePost("/trade/account/withdraws/generate_code", params), nil)
}
