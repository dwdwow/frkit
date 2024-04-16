package frbnc

import (
	"errors"
	"math"
	"sort"

	"github.com/dwdwow/cex/bnc"
	"github.com/dwdwow/mathy"
)

type RiskyLoanDemand struct {
	Order            bnc.CryptoLoanFlexibleOngoingOrder
	TargetLtv        float64
	TotalColltDemand float64
	AdditionalCollt  float64
	AdditionalUsd    float64
}

func AnalyzeLoan(acct *Account, ord bnc.CryptoLoanFlexibleOngoingOrder) (demand RiskyLoanDemand, risky bool) {
	switch {
	case ord.LoanCoin != "USDT", ord.CurrentLTV <= 0, ord.TotalDebt < 10:
		return
	}

	currtLtv := ord.CurrentLTV
	quality := qualityCollts[ord.CollateralCoin]
	high := (quality && currtLtv > maxQualityCollateralLtv) || (!quality && currtLtv > maxSubordinateCollateralLtv)
	if !high {
		return
	}
	targetLtv := middleSubordinateCollateralLtv
	if quality {
		targetLtv = middleQualityCollateralLtv
	}
	collDemand := ord.CollateralAmount * (currtLtv/targetLtv - 1)

	coll := ord.CollateralCoin
	bal, _ := acct.SpotBal(coll)
	ldbal, _ := acct.SpotBal("LD" + coll)
	sp := bal.Free + ldbal.Free

	addColl := mathy.RoundFloor(math.Min(collDemand, sp)*0.9999, 6)

	pColl := ord.TotalDebt / currtLtv / ord.CollateralAmount
	addUsdt := ord.TotalDebt - (ord.CollateralAmount+addColl)*pColl*targetLtv

	if addUsdt < 10 {
		addUsdt = 0
	}

	if addColl == 0 && addUsdt == 0 {
		return
	}

	demand = RiskyLoanDemand{
		Order:            ord,
		TargetLtv:        targetLtv,
		TotalColltDemand: collDemand,
		AdditionalCollt:  addColl,
		AdditionalUsd:    addUsdt,
	}

	return demand, true
}

type LoansAnalysis struct {
	Demands []RiskyLoanDemand
	Risky   bool
	Err     error
}

func AnalyzeRiskyLoans(acct *Account) LoansAnalysis {
	var demands []RiskyLoanDemand
	for _, ord := range acct.LoanOrders {
		result, risky := AnalyzeLoan(acct, ord)
		if risky {
			demands = append(demands, result)
		}
	}
	return LoansAnalysis{demands, len(demands) > 0, nil}
}

type FuturesUsdtAnalysis struct {
	WalletBalance float64
	Risky         bool
	Err           error
}

func AnalyzeFuturesUsdt(acct *Account) (analysis FuturesUsdtAnalysis) {
	usdt, ok := acct.FuAsset("USDT")
	if !ok {
		analysis.Err = errors.New("can not get usdt futures wallet balance")
		return
	}
	if usdt.WalletBalance > -5000 {
		return
	}
	analysis.WalletBalance = usdt.WalletBalance
	analysis.Risky = true
	return
}

type MarginableSpotBal struct {
	Coin            MarginCoin
	Qty             float64
	Price           float64
	MarginAvailable float64
	Err             error
}

func AnalyzeMarginableSpotBals(acct *Account) (bals []MarginableSpotBal) {
	for _, coin := range validMarginCoins {
		bal, ok := acct.SpotBal(coin.Coin)
		if !ok {
			continue
		}
		if bal.Free <= 0 {
			continue
		}
		symbol := coin.Coin + "USDT"
		fuPos, _ := acct.FuPos(symbol)
		price, err := fuPrice(fuPos)
		bals = append(bals, MarginableSpotBal{
			Coin:            coin,
			Qty:             bal.Free,
			Price:           price,
			MarginAvailable: bal.Free * price * coin.PledgeRatio,
			Err:             err,
		})
	}
	sort.Slice(bals, func(i, j int) bool {
		return bals[i].MarginAvailable > bals[j].MarginAvailable
	})
	return
}

type FuturesMarginAnalysis struct {
	CurrentMargin             float64
	CurrentTotalPos           float64
	CurrentMarginRatio        float64
	TargetMarginRatio         float64
	MarginRemand              float64
	MarginableSpotBals        []MarginableSpotBal
	TotalMarginableSpotsValue float64
	Risky                     bool
}

func AnalyzeMarginFutures(acct *Account) (analysis FuturesMarginAnalysis) {
	ratio, margin, totalPos := acct.MarginRatio()

	analysis.CurrentMargin = margin
	analysis.CurrentTotalPos = totalPos
	analysis.CurrentMarginRatio = ratio

	if ratio > minFuturesAccountMarginRatio {
		return
	}

	analysis.TargetMarginRatio = middleFuturesAccountMarginRatio

	marginRemand := (middleFuturesAccountMarginRatio - ratio) * totalPos

	analysis.MarginRemand = marginRemand

	if marginRemand <= 1 {
		return
	}

	analysis.Risky = true

	marginableSpotBals := AnalyzeMarginableSpotBals(acct)
	analysis.MarginableSpotBals = marginableSpotBals

	for _, bal := range marginableSpotBals {
		analysis.TotalMarginableSpotsValue += bal.MarginAvailable
	}

	return
}

type FuturesAnalysis struct {
	USDT   FuturesUsdtAnalysis
	Margin FuturesMarginAnalysis
	Risky  bool
	Err    error
}

func AnalyzeFutures(acct *Account) (analysis FuturesAnalysis) {
	usdt := AnalyzeFuturesUsdt(acct)
	margin := AnalyzeMarginFutures(acct)
	analysis.USDT = usdt
	analysis.Margin = margin
	analysis.Risky = usdt.Risky || margin.Risky
	return
}

type AccountAnalysis struct {
	Loans   LoansAnalysis
	Futures FuturesAnalysis
}

func AnalyzeAccount(acct *Account) (analysis AccountAnalysis) {
	analysis.Loans = AnalyzeRiskyLoans(acct)
	analysis.Futures = AnalyzeFutures(acct)
	return
}
