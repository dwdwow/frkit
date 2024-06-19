package frbnc

import (
	"time"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/go-resty/resty/v2"
)

const (
	WarnedUniMMR  = 3.0
	AlertedUniMMR = 2.0
)

type VIPPortmarAccountConfig struct {
	MinUniMMR      float64
	BalancedUniMMR float64
	MaxUniMMR      float64

	VIPLoanMaxLTV float64
}

type VIPPortmarAccount struct {
	ApiKey string          `json:"apiKey"`
	Time   int64           `json:"time"`
	Spot   bnc.SpotAccount `json:"spot"`

	PortmarAccountUMDetail    bnc.PortfolioMarginAccountDetail      `json:"portmarAccountUMDetail"`
	PortmarAccountCMDetail    bnc.PortfolioMarginAccountDetail      `json:"portmarAccountCMDetail"`
	PortmarAccountInformation bnc.PortfolioMarginAccountInformation `json:"portmarAccountInformation"`

	LoanOrders     []bnc.VIPLoanOngoingOrder          `json:"loanOrders"`
	LoanStatusInfo []bnc.VIPLoanApplicationStatusInfo `json:"loanStatusInfo"`

	PortmarCollateralRates []bnc.PortfolioMarginCollateralRate `json:"collateralRates"`

	spBals      map[string]bnc.SpotBalance
	pmAssets    map[string]bnc.PortfolioMarginAccountAsset
	pmPoss      map[string]bnc.PortfolioMarginAccountPosition
	pmCollRates map[string]bnc.PortfolioMarginCollateralRate
}

func (a VIPPortmarAccount) SpotBalance(asset string) (bnc.SpotBalance, bool) {
	return mapGetter(a.spBals, asset)
}

func (a VIPPortmarAccount) PortmarAsset(asset string) (bnc.PortfolioMarginAccountAsset, bool) {
	return mapGetter(a.pmAssets, asset)
}

func (a VIPPortmarAccount) PortmarPosition(symbol string) (bnc.PortfolioMarginAccountPosition, bool) {
	return mapGetter(a.pmPoss, symbol)
}

func (a VIPPortmarAccount) PortmarCollateralRate(asset string) (bnc.PortfolioMarginCollateralRate, bool) {
	return mapGetter(a.pmCollRates, asset)
}

func QueryVIPPortmarAccount(user *bnc.User) (resp *resty.Response, acct *VIPPortmarAccount, reqErr cex.RequestError) {
	resp, spot, reqErr := user.SpotAccount()
	if reqErr.IsNotNil() {
		return
	}
	resp, pmUMDetail, reqErr := user.PortfolioMarginAccountDetail()
	if reqErr.IsNotNil() {
		return
	}
	resp, pmCMDetail, reqErr := user.PortfolioMarginAccountCMDetail()
	if reqErr.IsNotNil() {
		return
	}
	resp, pmInfo, reqErr := user.PortfolioMarginAccountInformation()
	if reqErr.IsNotNil() {
		return
	}
	resp, loanOrders, reqErr := user.VIPLoanOngoingOrders("", "", "", "")
	if reqErr.IsNotNil() {
		return
	}
	resp, loanStatusInfo, reqErr := user.VIPLoanApplicationStatus()
	if reqErr.IsNotNil() {
		return
	}
	collRates, err := bnc.QueryPortfolioMarginCollateralRates()
	if err != nil {
		reqErr = cex.RequestError{Err: err}
		return
	}

	acct = &VIPPortmarAccount{
		ApiKey:                    user.Api().ApiKey,
		Time:                      time.Now().UnixMilli(),
		Spot:                      spot,
		PortmarAccountUMDetail:    pmUMDetail,
		PortmarAccountCMDetail:    pmCMDetail,
		PortmarAccountInformation: pmInfo,
		LoanOrders:                loanOrders.Rows,
		LoanStatusInfo:            loanStatusInfo.Rows,
		PortmarCollateralRates:    collRates,
		spBals:                    slice2map(spot.Balances, func(balance bnc.SpotBalance) string { return balance.Asset }),
		pmAssets:                  slice2map(pmUMDetail.Assets, func(asset bnc.PortfolioMarginAccountAsset) string { return asset.Asset }),
		pmPoss:                    slice2map(pmUMDetail.Positions, func(position bnc.PortfolioMarginAccountPosition) string { return position.Symbol }),
		pmCollRates: slice2map(collRates, func(rate bnc.PortfolioMarginCollateralRate) string {
			return rate.Asset
		}),
	}

	return
}
