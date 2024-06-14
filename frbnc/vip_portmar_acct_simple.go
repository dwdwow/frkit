package frbnc

import (
	"errors"
	"log/slog"
	"math"
	"slices"
	"sort"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/dwdwow/mathy"
)

type VIPPortmarAcctSimple struct {
	user   *bnc.User
	cfg    VIPPortmarAccountConfig
	chAcct chan VIPPortmarAcctWatcherMsg

	logger *slog.Logger
}

func (v *VIPPortmarAcctSimple) start() {
	for {
		msg := <-v.chAcct
		if msg.Err != nil {

		}
	}
}

func (v *VIPPortmarAcctSimple) handle(acct *VIPPortmarAccount) {}

type SpotCollInfo struct {
	Bal               bnc.SpotBalance
	Price             float64
	PmCollRate        float64
	LoanCollUsedValue float64
}

func (s SpotCollInfo) Value() float64 {
	return s.Bal.Free * s.Price
}

func (s SpotCollInfo) PmCollValue() float64 {
	return s.Value() * s.PmCollRate
}

func (v *VIPPortmarAcctSimple) handleMMR(acct *VIPPortmarAccount) {
	if acct.PortmarAccountInformation.UniMMR > v.cfg.MinUniMMR {
		return
	}

	deltaMMR := v.cfg.BalancedUniMMR - acct.PortmarAccountInformation.UniMMR

	equityNeed := acct.PortmarAccountInformation.AccountMaintMargin * deltaMMR

	remainingEquityNeed, err := v.handleLowMMR(acct, equityNeed)

	if err != nil {

	}

	// some perpetual should trade 50 usdt at least

	if remainingEquityNeed > 50 {

	}

}

func (v *VIPPortmarAcctSimple) handleLowMMR(acct *VIPPortmarAccount, equityNeed float64) (remainingEquityNeed float64, err error) {
	remainingEquityNeed = equityNeed

	_prices, err := bnc.QueryCMPremiumIndex("", "")

	// TODO should add some other price getters
	if err != nil {
		v.logger.Error("QueryCMPremiumIndex", "err", err)
		return
	}

	prices := slice2map(_prices, func(index bnc.CMPremiumIndex) string {
		return index.Symbol
	})

	var collInfos, suitableCollInfos []SpotCollInfo

	spots := acct.Spot.Balances
	for _, s := range spots {
		rate, ok := acct.PortmarCollateralRate(s.Asset)
		if !ok {
			v.logger.Error("No Portmar Collateral Rate", "asset", s.Asset)
			continue
		}
		price, ok := prices[s.Asset]
		if !ok {
			v.logger.Error("No Portmar Collateral Index Price Info", "asset", s.Asset)
			continue
		}
		collInfos = append(collInfos, SpotCollInfo{
			Bal:        s,
			Price:      price.IndexPrice,
			PmCollRate: rate.CollateralRate,
		})
	}

	sort.Slice(collInfos, func(i, j int) bool {
		collI := collInfos[i]
		collJ := collInfos[j]
		sybI := collI.Bal.Asset
		sybJ := collJ.Bal.Asset
		if sybI == "ETH" || sybI == "BTC" {
			return true
		}
		if sybJ == "ETH" || sybJ == "BTC" {
			return false
		}
		return collI.PmCollRate > collJ.PmCollRate
	})

	var totalDebt float64
	for _, ord := range acct.LoanOrders {
		totalDebt += ord.TotalDebt
	}

	minSpotCollValue := totalDebt / v.cfg.VIPLoanMaxLTV
	remainSpotCollValue := minSpotCollValue

	slices.Reverse(collInfos)
	for _, collInfo := range collInfos {
		value := collInfo.Value()
		if value < remainSpotCollValue {
			remainSpotCollValue -= value
			continue
		}
		if remainSpotCollValue > 0 {
			remainSpotCollValue = 0
			collInfo.LoanCollUsedValue = remainSpotCollValue
		}
		suitableCollInfos = append(suitableCollInfos, collInfo)
	}

	if len(suitableCollInfos) <= 0 {
		// TODO Dangerous Situation
		v.logger.Error("Spot Suitable Collaterals Are Not Enough")
		err = errors.New("spot suitable collaterals are enough")
		return
	}

	slices.Reverse(suitableCollInfos)

	const maxRemainNeed = 10

	for _, collInfo := range suitableCollInfos {
		if remainingEquityNeed < maxRemainNeed {
			break
		}
		value := collInfo.PmCollValue()
		transValue := math.Min(value, remainSpotCollValue)
		transQty := collInfo.Bal.Free * math.Min(1, transValue/value)
		transQty = mathy.RoundFloor(transQty, 6)
		_, transRes, reqErr := v.user.Transfer(bnc.TransferTypeMainPortfolioMargin, collInfo.Bal.Asset, transQty)
		if reqErr.IsNotNil() {
			v.logger.Error("Transfer Main -> PM Account Failed", "asset", collInfo.Bal.Asset, "qyt", transQty, "err", reqErr.Err)
			continue
		}
		v.logger.Info("Main -> PM Account", "asset", collInfo.Bal.Asset, "qyt", transQty, "result", transRes)
		remainingEquityNeed -= transValue
	}

	if remainingEquityNeed >= maxRemainNeed {
		v.logger.Warn("Remaining Equity Not Enough", "remainingEquityNeed", remainingEquityNeed)
	}

	return
}

func (v *VIPPortmarAcctSimple) cutPositions(acct *VIPPortmarAccount, equityNeed float64) (remainingEquityNeed float64, err error) {
	return
}

func (v *VIPPortmarAcctSimple) cutPosition(spotPair, fuPair cex.Pair, qty float64) error {
	return nil
}

func (v *VIPPortmarAcctSimple) handleHighLtv(acct *VIPPortmarAccount) {}
