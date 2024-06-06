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
	MinUniMMR          float64
	BalancedUniMMR     float64
	MaxUniMMR          float64
	MinVIPLoanLTV      float64
	BalancedVIPLoanLTV float64
	MaxVIPLoanLTV      float64
}

type VIPPortmarAccount struct {
	ApiKey string          `json:"apiKey"`
	Time   int64           `json:"time"`
	Spot   bnc.SpotAccount `json:"spot"`

	PortmarAccountDetail      bnc.PortfolioMarginAccountDetail      `json:"portmarAccountDetail"`
	PortmarAccountInformation bnc.PortfolioMarginAccountInformation `json:"portmarAccountInformation"`
	PortMarAccountBalances    []bnc.PortfolioMarginBalance          `json:"portMarAccountBalances"`
	PortMarAccountUMPositions []bnc.PortfolioMarginUMPositionRisk   `json:"portMarAccountUMPositions"`
	PortMarAccountCMPositions []bnc.PortfolioMarginCMPositionRisk   `json:"portMarAccountCMPositions"`
	PortMarCollateralRates    []bnc.PortfolioMarginCollateralRate   `json:"portMarCollateralRates"`

	LoanOrders           []bnc.VIPLoanOngoingOrder          `json:"loanOrders"`
	LoanStatusInfo       []bnc.VIPLoanApplicationStatusInfo `json:"loanStatusInfo"`
	LoanCollateralAssets []bnc.VIPLoanCollateralAsset       `json:"loanCollateralAssets"`

	// spot account
	spBals map[string]bnc.SpotBalance
	// cross margin account
	pmBals map[string]bnc.PortfolioMarginBalance
	// um futures account
	umPoss map[string]bnc.PortfolioMarginUMPositionRisk
	// cm futures account
	cmPoss map[string]bnc.PortfolioMarginCMPositionRisk

	pmAssets       map[string]bnc.PortfolioMarginAccountAsset
	pmPoss         map[string]bnc.PortfolioMarginAccountPosition
	pmCollRateMap  map[string]float64
	loanCollAssets map[string]bnc.VIPLoanCollateralAsset

	spPriceMap map[string]float64
	umPriceMap map[string]float64
	cmPriceMap map[string]float64
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

func (a VIPPortmarAccount) PortmarCollateralRate(asset string) (float64, bool) {
	return mapGetter(a.pmCollRateMap, asset)
}

func QueryVIPPortmarAccount(user *bnc.User) (resp *resty.Response, acct *VIPPortmarAccount, reqErr cex.RequestError) {
	resp, spot, reqErr := user.SpotAccount()
	if reqErr.IsNotNil() {
		return
	}

	resp, pmDetail, reqErr := user.PortfolioMarginAccountDetail()
	if reqErr.IsNotNil() {
		return
	}

	resp, pmInfo, reqErr := user.PortfolioMarginAccountInformation()
	if reqErr.IsNotNil() {
		return
	}

	_, pmBals, reqErr := user.PortfolioMarginBalances()
	if reqErr.IsNotNil() {
		return
	}

	_, pmPoss, reqErr := user.PortfolioMarginPositions("")
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

	pmCollRates, err := bnc.QueryPortfolioMarginCollateralRates()
	if err != nil {
		reqErr = cex.RequestError{Err: err}
		return
	}

	spPrices, err := bnc.QuerySpotPrices()
	if err != nil {
		reqErr = cex.RequestError{Err: err}
		return
	}

	fuPrices, err := bnc.QueryFuturesPrices()
	if err != nil {
		reqErr = cex.RequestError{Err: err}
		return
	}

	acct = &VIPPortmarAccount{
		ApiKey: user.Api().ApiKey,
		Time:   time.Now().UnixMilli(),
		Spot:   spot,

		PortmarAccountDetail:      pmDetail,
		PortmarAccountInformation: pmInfo,
		PortMarAccountBalances:    pmBals,
		PortMarAccountUMPositions: pmPoss,
		PortMarAccountCMPositions: []bnc.PortfolioMarginCMPositionRisk{},
		PortMarCollateralRates:    pmCollRates,

		LoanOrders:           loanOrders.Rows,
		LoanStatusInfo:       loanStatusInfo.Rows,
		LoanCollateralAssets: []bnc.VIPLoanCollateralAsset{},

		spBals: slice2map(spot.Balances, func(balance bnc.SpotBalance) string { return balance.Asset }),

		pmBals: slice2map(pmBals, func(balance bnc.PortfolioMarginBalance) string { return balance.Asset }),

		umPoss: slice2map(pmPoss, func(position bnc.PortfolioMarginUMPositionRisk) string { return position.Symbol }),

		cmPoss: map[string]bnc.PortfolioMarginCMPositionRisk{},

		pmAssets: slice2map(pmDetail.Assets, func(asset bnc.PortfolioMarginAccountAsset) string { return asset.Asset }),
		pmPoss:   slice2map(pmDetail.Positions, func(position bnc.PortfolioMarginAccountPosition) string { return position.Symbol }),
		pmCollRateMap: slice2mapkv(pmCollRates, func(rate bnc.PortfolioMarginCollateralRate) string {
			return rate.Asset
		}, func(rate bnc.PortfolioMarginCollateralRate) float64 {
			return rate.CollateralRate
		}),
		loanCollAssets: map[string]bnc.VIPLoanCollateralAsset{},

		spPriceMap: slice2mapkv(spPrices, func(price bnc.SpotPriceTicker) string { return price.Symbol }, func(price bnc.SpotPriceTicker) float64 { return price.Price }),
		umPriceMap: slice2mapkv(fuPrices, func(price bnc.FuturesPriceTicker) string { return price.Symbol }, func(price bnc.FuturesPriceTicker) float64 { return price.Price }),
		cmPriceMap: map[string]float64{},
	}

	return
}
