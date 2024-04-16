package frbnc

import (
	"time"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/go-resty/resty/v2"
)

type Account struct {
	ApiKey        string                               `json:"apiKey"`
	Time          int64                                `json:"time"`
	Spot          bnc.SpotAccount                      `json:"spot"`
	Futures       bnc.FuturesAccount                   `json:"futures"`
	EarnPositions []bnc.SimpleEarnFlexiblePosition     `json:"earnPositions"`
	LoanOrders    []bnc.CryptoLoanFlexibleOngoingOrder `json:"loanOrders"`

	spBals map[string]bnc.SpotBalance
	fuAsts map[string]bnc.FuturesAccountAsset
	fuPoss map[string]bnc.FuturesAccountPosition
	enPoss map[string]bnc.SimpleEarnFlexiblePosition
	lnOrds map[string]bnc.CryptoLoanFlexibleOngoingOrder // key is loanCoin+"_"+collateralCoin
}

func (a *Account) EarnProductId(asset string) (id string, ok bool) {
	if a.enPoss == nil {
		return
	}
	p, ok := a.enPoss[asset]
	return p.ProductId, ok
}

func (a *Account) SpotBal(asset string) (bal bnc.SpotBalance, ok bool) {
	if a.spBals == nil {
		return
	}
	bal, ok = a.spBals[asset]
	return
}

func (a *Account) FuAsset(asset string) (ass bnc.FuturesAccountAsset, ok bool) {
	if a.fuAsts == nil {
		return
	}
	ass, ok = a.fuAsts[asset]
	return
}

func (a *Account) FuPos(symbol string) (pos bnc.FuturesAccountPosition, ok bool) {
	if a.fuPoss == nil {
		return
	}
	pos, ok = a.fuPoss[symbol]
	return
}

func (a *Account) EarnPos(asset string) (pos bnc.SimpleEarnFlexiblePosition, ok bool) {
	if a.enPoss == nil {
		return
	}
	pos, ok = a.enPoss[asset]
	return
}

// LoanOrd
// pair is loanCoin+"_"+collateralCoin
func (a *Account) LoanOrd(pair string) (ord bnc.CryptoLoanFlexibleOngoingOrder, ok bool) {
	if a.lnOrds == nil {
		return
	}
	ord, ok = a.lnOrds[pair]
	return
}

// MarginRatio returns totalMarginBalance / totalPos
// If return 0, totalPos = 0
func (a *Account) MarginRatio() (ratio, margin, totalPos float64) {
	acct := a.Futures
	margin = acct.TotalMarginBalance

	for _, pos := range acct.Positions {
		totalPos += pos.PositionInitialMargin * pos.Leverage
	}

	if totalPos <= 0 {
		return
	}

	ratio = margin / totalPos

	return
}

func QueryAccount(user *bnc.User) (resp *resty.Response, acct *Account, err cex.RequestError) {
	resp, spot, err := user.SpotAccount()
	if err.IsNotNil() {
		return
	}
	resp, futures, err := user.FuturesAccount()
	if err.IsNotNil() {
		return
	}
	resp, poss, err := user.SimpleEarnFlexiblePositions("", "")
	if err.IsNotNil() {
		return
	}
	resp, ords, err := user.CryptoLoanFlexibleOngoingOrders("", "")
	if err.IsNotNil() {
		return
	}
	return resp, &Account{
		ApiKey:        user.Api().ApiKey,
		Time:          time.Now().UnixMilli(),
		Spot:          spot,
		Futures:       futures,
		EarnPositions: poss.Rows,
		LoanOrders:    ords.Rows,

		spBals: slice2map(spot.Balances, func(bal bnc.SpotBalance) string {
			return bal.Asset
		}),
		fuAsts: slice2map(futures.Assets, func(asset bnc.FuturesAccountAsset) string {
			return asset.Asset
		}),
		fuPoss: slice2map(futures.Positions, func(pos bnc.FuturesAccountPosition) string {
			return pos.Symbol
		}),
		enPoss: slice2map(poss.Rows, func(pos bnc.SimpleEarnFlexiblePosition) string {
			return pos.Asset
		}),
		lnOrds: slice2map(ords.Rows, func(ord bnc.CryptoLoanFlexibleOngoingOrder) string {
			return ord.LoanCoin + "_" + ord.CollateralCoin
		}),
	}, cex.RequestError{}
}
