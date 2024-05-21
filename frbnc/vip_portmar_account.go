package frbnc

import (
	"time"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/go-resty/resty/v2"
)

type VIPPortmarAccount struct {
	ApiKey string          `json:"apiKey"`
	Time   int64           `json:"time"`
	Spot   bnc.SpotAccount `json:"spot"`

	PortmarAccountDetail      bnc.PortfolioMarginAccountDetail      `json:"portmarAccountDetail"`
	PortmarAccountInformation bnc.PortfolioMarginAccountInformation `json:"portmarAccountInformation"`

	LoanOrders     []bnc.VIPLoanOngoingOrder          `json:"loanOrders"`
	LoanStatusInfo []bnc.VIPLoanApplicationStatusInfo `json:"loanStatusInfo"`

	spBals   map[string]bnc.SpotBalance
	pmAssets map[string]bnc.PortfolioMarginAccountAsset
	pmPoss   map[string]bnc.PortfolioMarginAccountPosition
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
	resp, loanOrders, reqErr := user.VIPLoanOngoingOrders(0, 0, "", "")
	if reqErr.IsNotNil() {
		return
	}
	resp, loanStatusInfo, reqErr := user.VIPLoanApplicationStatus()
	if reqErr.IsNotNil() {
		return
	}

	acct = &VIPPortmarAccount{
		ApiKey:                    user.Api().ApiKey,
		Time:                      time.Now().UnixMilli(),
		Spot:                      spot,
		PortmarAccountDetail:      pmDetail,
		PortmarAccountInformation: pmInfo,
		LoanOrders:                loanOrders.Rows,
		LoanStatusInfo:            loanStatusInfo.Rows,
		spBals:                    slice2map(spot.Balances, func(balance bnc.SpotBalance) string { return balance.Asset }),
		pmAssets:                  slice2map(pmDetail.Assets, func(asset bnc.PortfolioMarginAccountAsset) string { return asset.Asset }),
		pmPoss:                    slice2map(pmDetail.Positions, func(position bnc.PortfolioMarginAccountPosition) string { return position.Symbol }),
	}

	return
}
