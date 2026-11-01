package safetrade

const AllMarkets = "all"

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type OrderType string

const (
	OrderTypeLimit       OrderType = "limit"
	OrderTypeMarket      OrderType = "market"
	OrderTypeMarketQuote OrderType = "market_quote"
	OrderTypeStopLimit   OrderType = "stop_limit"
)

type OrderState string

const (
	OrderStatePending  OrderState = "pending"
	OrderStateWait     OrderState = "wait"
	OrderStateDone     OrderState = "done"
	OrderStateCancel   OrderState = "cancel"
	OrderStateRejected OrderState = "rejected"
)

type Ordering string

const (
	OrderingAsc  Ordering = "asc"
	OrderingDesc Ordering = "desc"
)

type AccountType string

const (
	AccountTypeSpot   AccountType = "spot"
	AccountTypeMargin AccountType = "margin"
)

type DepositStatus string

const (
	DepositStatusSubmitted     DepositStatus = "submitted"
	DepositStatusCanceled      DepositStatus = "canceled"
	DepositStatusRejected      DepositStatus = "rejected"
	DepositStatusAccepted      DepositStatus = "accepted"
	DepositStatusProcessing    DepositStatus = "processing"
	DepositStatusSkipped       DepositStatus = "skipped"
	DepositStatusCollecting    DepositStatus = "collecting"
	DepositStatusCollected     DepositStatus = "collected"
	DepositStatusFeeCollecting DepositStatus = "fee_collecting"
	DepositStatusFeeProcessing DepositStatus = "fee_processing"
	DepositStatusFeeCollected  DepositStatus = "fee_collected"
	DepositStatusErrored       DepositStatus = "errored"
)

type WithdrawalStatus string

const (
	WithdrawalStatusPrepared    WithdrawalStatus = "prepared"
	WithdrawalStatusCanceled    WithdrawalStatus = "canceled"
	WithdrawalStatusAccepted    WithdrawalStatus = "accepted"
	WithdrawalStatusSkipped     WithdrawalStatus = "skipped"
	WithdrawalStatusToReject    WithdrawalStatus = "to_reject"
	WithdrawalStatusRejected    WithdrawalStatus = "rejected"
	WithdrawalStatusProcessing  WithdrawalStatus = "processing"
	WithdrawalStatusUnderReview WithdrawalStatus = "under_review"
	WithdrawalStatusSucceed     WithdrawalStatus = "succeed"
	WithdrawalStatusFailed      WithdrawalStatus = "failed"
	WithdrawalStatusErrored     WithdrawalStatus = "errored"
	WithdrawalStatusConfirming  WithdrawalStatus = "confirming"
)

type WithdrawalType string

const (
	WithdrawalTypeOnchain WithdrawalType = "onchain"

	WithdrawalTypeInternal WithdrawalType = "internal"
)

type BeneficiaryState string

const (
	BeneficiaryStatePending  BeneficiaryState = "pending"
	BeneficiaryStateActive   BeneficiaryState = "active"
	BeneficiaryStateDisabled BeneficiaryState = "disabled"
)

type CodeChannel string

const (
	CodeChannelEmail CodeChannel = "email"
	CodeChannelPhone CodeChannel = "phone"
)

type TickerPeriod string

const (
	TickerPeriod24h  TickerPeriod = "24h"
	TickerPeriod48h  TickerPeriod = "48h"
	TickerPeriod168h TickerPeriod = "168h"
	TickerPeriod720h TickerPeriod = "720h"
)

type DistributionPeriod string

const (
	DistributionPeriod6h  DistributionPeriod = "6h"
	DistributionPeriod12h DistributionPeriod = "12h"
	DistributionPeriod24h DistributionPeriod = "24h"
)

type PriceHistoryPeriod string

const (
	PriceHistoryDay         PriceHistoryPeriod = "24h"
	PriceHistoryWeek        PriceHistoryPeriod = "7d"
	PriceHistoryMonth       PriceHistoryPeriod = "1m"
	PriceHistoryThreeMonths PriceHistoryPeriod = "3m"
	PriceHistorySixMonths   PriceHistoryPeriod = "6m"
	PriceHistoryYear        PriceHistoryPeriod = "1y"
	PriceHistoryAll         PriceHistoryPeriod = "all"
)
