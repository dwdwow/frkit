package frbnc

import (
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/dwdwow/cex/bnc"
)

type VIPPortmarAcctSimple struct {
	user   *bnc.User
	cfg    VIPPortmarAccountConfig
	chAcct chan VIPPortmarAcctWatcherMsg
}

func (v *VIPPortmarAcctSimple) start() {
	for msg := range v.chAcct {
		if msg.Err != nil {
			slog.Error(msg.Err.Error())
			continue
		}

		if time.Now().UnixMilli()-msg.Acct.Time > 2000 {
			slog.Error("Account data too old")
			continue
		}

		v.handle(msg.Acct)
	}
}

func (v *VIPPortmarAcctSimple) handle(acct *VIPPortmarAccount) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go v.handleLowMMR(wg, acct)
	go v.handleHighLtv(wg, acct)
	wg.Wait()
}

func (v *VIPPortmarAcctSimple) handleLowMMR(wg *sync.WaitGroup, acct *VIPPortmarAccount) {
	defer wg.Done()

	pmAcctInfo := acct.PortmarAccountInformation

	// check uniMMR
	if pmAcctInfo.UniMMR > v.cfg.MinUniMMR {
		return
	}

	needUSDTAmount := pmAcctInfo.AccountMaintMargin*v.cfg.BalancedUniMMR - pmAcctInfo.AccountEquity

	// add pm collateral
	if err := addPortmarCollateral(v.user, acct, needUSDTAmount); err != nil {
		return
	}
}

func addPortmarCollateral(user *bnc.User, acct *VIPPortmarAccount, needUSDTAmount float64) error {
	if needUSDTAmount <= 1e2 {
		return fmt.Errorf("needUSDTAmount too small: %f", needUSDTAmount)
	}

	var availUSDTAmount float64
	switch len(acct.LoanOrders) {
	case 0:
		// no loan order, trasfer from spot to portmar
		// availUSDTAmount = +Inf is appropriate
		// 1e4 here is a random cap, need to be modified later
		availUSDTAmount = 1e4
	case 1:
		// has only one loan order, simplify the logic
		vipLoanOrder := acct.LoanOrders[0]
		availUSDTAmount = math.Max((vipLoanOrder.TotalCollateralValueAfterHaircut-vipLoanOrder.LockedCollateralValue)*0.99, 0)
	default:
		// need implementation
		// if has multiple USDT loan orders, same as one order
		// if has non-USDT loan orders, return
		for _, ord := range acct.LoanOrders {
			if ord.LoanCoin != "USDT" {
				return fmt.Errorf("unexpected loan coin: %s", ord.LoanCoin)
			}
		}
		vipLoanOrder := acct.LoanOrders[0]
		availUSDTAmount = math.Max((vipLoanOrder.TotalCollateralValueAfterHaircut-vipLoanOrder.LockedCollateralValue)*0.99, 0)
	}

	// sort spot assets by collateral ratio in descending order
	spotBals := delSpotAcctAssetZeroValue(acct)
	pmCollateralRateMap := acct.pmCollRateMap

	slices.SortFunc(spotBals, func(i, j bnc.SpotBalance) int {
		switch {
		case pmCollateralRateMap[i.Asset] > pmCollateralRateMap[j.Asset]:
			return -1
		case pmCollateralRateMap[i.Asset] < pmCollateralRateMap[j.Asset]:
			return 1
		default:
			return 0
		}
	})

	for _, sBal := range spotBals {
		collateralRate, ok := pmCollateralRateMap[sBal.Asset]
		if !ok || collateralRate <= 0.2 {
			continue
		}
		price := acct.spPriceMap[sBal.Asset+"USDT"]

		// transferCoinAmount needs precision control
		transferCoinAmount := math.Min(math.Min(sBal.Free, availUSDTAmount/price), needUSDTAmount/collateralRate/price)
		if transferCoinAmount*price < 10 {
			continue
		}

		time.Sleep(time.Second)
		if _, _, err := user.Transfer(
			bnc.TransferTypeMainMargin,
			sBal.Asset,
			transferCoinAmount,
		); err.IsNotNil() {
			slog.Error(err.Error())
			continue
		}

		availUSDTAmount -= transferCoinAmount * price
		needUSDTAmount -= transferCoinAmount * collateralRate * price

		if availUSDTAmount < 10 && needUSDTAmount > 1e3 {
			return fmt.Errorf("needUSDTAmount not fully covered: %f", needUSDTAmount)
		}

		if needUSDTAmount < 10 {
			return nil
		}
	}

	return nil
}

func (v *VIPPortmarAcctSimple) handleHighLtv(wg *sync.WaitGroup, acct *VIPPortmarAccount) {
	defer wg.Done()

	//acct.LoanOrders
	var vipLoanOrder *bnc.VIPLoanOngoingOrder
	switch len(acct.LoanOrders) {
	case 0:
		// no loan order, do nothing
		return
	case 1:
		// has only one loan order, simplify the logic
		vipLoanOrder = &acct.LoanOrders[0]
		if vipLoanOrder.LoanCoin != "USDT" {
			return
		}
	default:
		for _, ord := range acct.LoanOrders {
			if ord.LoanCoin != "USDT" {
				return
			}
		}
		vipLoanOrder = &acct.LoanOrders[0]
	}

	if vipLoanOrder.CurrentLTV < v.cfg.MaxVIPLoanLTV {
		return
	}

	needUSDTAmount := vipLoanOrder.TotalCollateralValueAfterHaircut * (vipLoanOrder.CurrentLTV/v.cfg.BalancedVIPLoanLTV - 1)

	// add sp collateral
	if err := addSPCollateral(v.user, acct, needUSDTAmount); err != nil {
		slog.Error(err.Error())
	}
}

func addSPCollateral(user *bnc.User, acct *VIPPortmarAccount, needUSDTAmount float64) error {
	if needUSDTAmount < 1e2 {
		return fmt.Errorf("needUSDTAmount too small: %f", needUSDTAmount)
	}

	if acct.PortmarAccountInformation.UniMMR < 10 {
		return fmt.Errorf("UniMMR too low: %f", acct.PortmarAccountInformation.UniMMR)
	}

	availUSDTAmount := acct.PortmarAccountInformation.VirtualMaxWithdrawAmount

	for _, pBal := range acct.PortMarAccountBalances {
		if _, ok := acct.loanCollAssets[pBal.Asset]; !ok {
			continue
		}

		price := acct.spPriceMap[pBal.Asset+"USDT"]
		transferCoinAmount := math.Min(pBal.CrossMarginFree, math.Min(needUSDTAmount, availUSDTAmount)/price)
		if transferCoinAmount*price < 10 {
			continue
		}

		time.Sleep(time.Second)
		if _, _, err := user.Transfer(bnc.TransferTypeMarginMain, pBal.Asset, transferCoinAmount); err.IsNotNil() {
			slog.Error(err.Error())
			continue
		}

		availUSDTAmount -= transferCoinAmount * price
		needUSDTAmount -= transferCoinAmount * price

		if availUSDTAmount < 10 && needUSDTAmount > 1e3 {
			return fmt.Errorf("needUSDTAmount not fully covered: %f", needUSDTAmount)
		}

		if needUSDTAmount < 10 {
			return nil
		}
	}

	return nil
}
