package frbnc

import (
	"log/slog"
	"slices"
	"sort"

	"github.com/dwdwow/cex/bnc"
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

func (v *VIPPortmarAcctSimple) handleLowMMR(acct *VIPPortmarAccount) {
	if acct.PortmarAccountInformation.UniMMR > v.cfg.MinUniMMR {
		return
	}

	deltaMMR := v.cfg.BalancedUniMMR - acct.PortmarAccountInformation.UniMMR

	equityNeed := acct.PortmarAccountInformation.AccountMaintMargin * deltaMMR

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
			continue
		}
		price, ok := prices[s.Asset]
		if !ok {
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
		return
	}

	slices.Reverse(suitableCollInfos)

	remainingEquityNeed := equityNeed

	_ = remainingEquityNeed

}

func (v *VIPPortmarAcctSimple) handleHighLtv(acct *VIPPortmarAccount) {}
