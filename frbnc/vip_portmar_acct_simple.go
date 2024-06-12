package frbnc

import (
	"log/slog"

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

	var crrLtv, spotCollCap float64
	if len(acct.LoanOrders) > 0 {
		ord := acct.LoanOrders[0]
		crrLtv = ord.CurrentLTV

		if crrLtv >= v.cfg.VIPLoanMaxLTV {
			v.logger.Error("Spot LTV Is Too High, Cannot Transfer Spots To Margin Portfolio Account", "ltv", crrLtv)
			return
		}

		// TODO collateral rates need be considered
		spotCollCap = ord.TotalCollateralValueAfterHaircut - ord.TotalDebt/v.cfg.VIPLoanMaxLTV
	} else {
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
			spotCollCap += s.Free * price.IndexPrice * rate.CollateralRate
		}
	}

	if spotCollCap < equityNeed {
		v.logger.Warn("Spot Collateral Value Is Not Enough", "cap", spotCollCap, "need", equityNeed)
	}

}

func (v *VIPPortmarAcctSimple) handleHighLtv(acct *VIPPortmarAccount) {}
