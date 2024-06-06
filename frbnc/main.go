package frbnc

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/dwdwow/mathy"
	"github.com/dwdwow/props"
)

type Main struct {
	user        *bnc.User
	acctWatcher *AcctWatcher

	muxHandling sync.Mutex

	logger *slog.Logger
}

func NewMain(user *bnc.User, logger *slog.Logger) (*Main, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	logger = logger.With("cex", user.Api().Cex, "apiKey", user.Api().ApiKey)
	watcher := NewAcctWatcher(user, logger)
	// TODO
	//err := watcher.Start()
	//if err != nil {
	//	return nil, err
	//}
	return &Main{
		user:        user,
		acctWatcher: watcher,
		logger:      logger,
	}, nil
}

func (m *Main) wait() {
	suber := m.acctWatcher.Sub()
	for acct := range suber {
		if acct.Err != nil {
			m.logger.Error("Receive Account Error", "err", acct.Err)
			continue
		}
		go m.handle(acct.Acct)
	}
}

func (m *Main) handle(acct *Account) {
	if !m.muxHandling.TryLock() {
		return
	}
	defer m.muxHandling.Unlock()

	m.handleRedundant(acct)

	m.handleAnalysis(acct, AnalyzeAccount(acct))
}

func (m *Main) handleRedundant(acct *Account) {
	m.handleLowLtvOrds(acct)
	m.adjustLowRiskFutureAccount(acct)
}

func (m *Main) handleLowLtvOrds(acct *Account) {
	lowOrds, _ := m.ClassifyLoanOrds(acct)
	results, errs := m.AdjustLowLtvLoanOrds(m.user, lowOrds)
	for _, res := range results {
		m.logger.Info("Low Ltv Order Adjusted", "result", res)
	}
	for _, err := range errs {
		m.logger.Error("Cannot Adjust Low Ltv Order", "err", err)
	}
	updating, _, err := m.acctWatcher.Update()
	if err != nil {
		m.logger.Error("Cannot Update Account", "err", err)
	}
	if updating {
		m.logger.Warn("Account Is Updating")
		time.Sleep(time.Second)
		_, _, err := m.acctWatcher.Update()
		if err != nil {
			m.logger.Error("Cannot Update Account", "err", err)
		}
	}
}

func (m *Main) adjustLowRiskFutureAccount(acct *Account) {
	_, marginValue, totalPos := acct.MarginRatio()

	var marginGap float64

	if totalPos <= 0 {
		m.logger.Info("Future Total Position Is 0")
		return
	} else {
		marginGap = totalPos * math.Abs(marginValue/totalPos-middleFuturesAccountMarginRatio)
	}

	remainMarginGap := marginGap

	var validCoins []string
	var usdtAsset bnc.FuturesAccountAsset
	for _, asset := range acct.fuAsts {
		// Here must be 0,
		// because is asset amount, not USDT,
		// be careful.
		if asset.MaxWithdrawAmount <= 0 {
			continue
		}

		coin := asset.Asset

		if coin == "USDT" {
			usdtAsset = asset
			continue
		}

		validCoins = append(validCoins, coin)
	}

	if usdtAsset.MaxWithdrawAmount > 1 {
		validCoins = slices.Insert(validCoins, 0, usdtAsset.Asset)
	}

	_acct := acct

	for _, coin := range validCoins {
		// The asset status may change,
		// so should get new asset timely.
		asset, ok := _acct.FuAsset(coin)
		if !ok {
			continue
		}

		widrValue, tranRes, err := m.ReduceFuCollats(m.user, acct, asset, remainMarginGap)

		if err != nil {
			m.logger.Error("Cannot Reduce Futures Collaterals", "err", err)
			continue
		}

		m.logger.Info("Futures Collateral Reduced", "result", tranRes)

		remainMarginGap -= widrValue
		if remainMarginGap <= 10 {
			break
		}

		if widrValue > 0 {
			// Should sleep 2 seconds, avoid high frequency.
			time.Sleep(time.Second * 2)

			// The withdrawal action may change other assets' maxWithdrawAmount,
			// so should query future assets again.
			// Query future account info may result in high frequency,
			// so can calculate the influence of previous withdrawal action.
			_, newAcct, err := m.acctWatcher.Update()
			if err != nil {
				m.logger.Error("Cannot Update Account", "err", err)
				continue
			}
			_acct = newAcct
		}
	}

	if remainMarginGap > 10 {
		m.logger.Error("Futures Collateral Remaining Value > 10")
	}
}

func (m *Main) handleAnalysis(acct *Account, analysis AccountAnalysis) {
	loanAnaly := analysis.Loans
	fuAnaly := analysis.Futures
	switch {
	case loanAnaly.Risky && fuAnaly.Risky:
		m.handleBothRisky(acct, analysis)
	case loanAnaly.Risky:
		m.handleLoanRisky(acct, analysis)
	case fuAnaly.Risky:
		m.handleFuRisky(acct, analysis)
	}
}

func (m *Main) handleBothRisky(acct *Account, analysis AccountAnalysis) {
	// TODO
}

func (m *Main) handleLoanRisky(acct *Account, analysis AccountAnalysis) {

}

func (m *Main) handleFuRisky(acct *Account, analysis AccountAnalysis) {

}

func (m *Main) ClassifyLoanOrds(acct *Account) (lowLtvOrds, highLtvOrds []bnc.CryptoLoanFlexibleOngoingOrder) {
	ords := acct.lnOrds

	for _, ord := range ords {
		if ord.LoanCoin != "USDT" || ord.TotalDebt < 20 {
			continue
		}
		ltv := ord.CurrentLTV
		isHighLevel := qualityCollts[ord.CollateralCoin]
		if (isHighLevel && ltv < minQualityCollateralLtv) || (!isHighLevel && ltv < minSubordinateCollateralLtv) {
			lowLtvOrds = append(lowLtvOrds, ord)
		} else if (isHighLevel && ltv > maxQualityCollateralLtv) || (!isHighLevel && ltv > maxSubordinateCollateralLtv) {
			highLtvOrds = append(highLtvOrds, ord)
		}
	}

	slices.SortFunc(lowLtvOrds, func(i, j bnc.CryptoLoanFlexibleOngoingOrder) int {
		return int(math.Copysign(1, i.CurrentLTV-j.CurrentLTV))
	})

	slices.SortFunc(highLtvOrds, func(i, j bnc.CryptoLoanFlexibleOngoingOrder) int {
		return int(math.Copysign(1, j.CurrentLTV-i.CurrentLTV))
	})

	//var lowCollaterals, highCollaterals []string
	//
	//for _, ord := range lowLtvOrds {
	//	lowCollaterals = append(lowCollaterals, ord.CollateralCoin)
	//}
	//
	//for _, ord := range highLtvOrds {
	//	highCollaterals = append(highCollaterals, ord.CollateralCoin)
	//}
	return
}

//func (m *Main ReduceAndRedeemLoanOrdCollateralCoin(user *bnc.User, loanCoin, collateralCoin string, adjAmt float64) error {
//	_, adRes, err := user.CryptoLoanFlexibleAdjustLtv(loanCoin, collateralCoin, adjAmt, bnc.LTVReduced)
//	if err.IsNotNil() {
//		return err.Err
//	}
//
//	if c.querySimpleEarnPoss(flow).Stopped() {
//		return flow.GetStatus()
//	}
//
//	if c.redeem(flow).Stopped() {
//		flow.SetStatus(FlowWarn)
//	}
//
//	return flow.GetStatus()
//
//	// after adjusting, should wait for seconds,
//	// but do not know how many seconds,
//	// so should retry 3 times
//	//flow.Info("Redeeming all", collateralCoin)
//	//resp, result, err := c.user.SimpleEarnFlexibleRedeem(collateralCoin, true, 0, bnc.SimpleEarnFlexibleRedeemDestinationSpot, cex.CltOptRetryCount(3, time.Second*2))
//	//if err.IsNotNil() {
//	//	return flow.ReqErr(resp, err, 1, "Cannot redeem all", collateralCoin)
//	//}
//	//flow.Info("All", collateralCoin, "redeemed,", result)
//	//return flow.GetStatus()
//}

func (m *Main) AdjustLowLtvLoanOrds(user *bnc.User, ords []bnc.CryptoLoanFlexibleOngoingOrder) (adResults []bnc.CryptoLoanFlexibleLoanAdjustLtvResult, errs []error) {
	for i, ord := range ords {
		ltv0 := ord.CurrentLTV
		var ltv1 float64
		if qualityCollts[ord.CollateralCoin] {
			ltv1 = middleQualityCollateralLtv
		} else {
			ltv1 = middleSubordinateCollateralLtv
		}

		redunColl := ord.CollateralAmount * (1 - ltv0/ltv1)
		redunColl = mathy.RoundFloor(redunColl, 5)

		_, adRes, err := user.CryptoLoanFlexibleAdjustLtv(ord.LoanCoin, ord.CollateralCoin, redunColl, bnc.LTVReduced)

		if err.IsNotNil() {
			errs = append(errs, err.Err)
			adResults = append(adResults, adRes)
		} else {
			if i != len(ords)-1 {
				time.Sleep(time.Second * 2)
			}
		}
	}
	return
}

func (m *Main) ReduceFuCollats(user *bnc.User, acct *Account, asset bnc.FuturesAccountAsset, shouldWidrValue float64) (finalWidrValue float64, tranRes bnc.UniversalTransferResp, err error) {
	coin := asset.Asset
	maxWidrQty := asset.MaxWithdrawAmount
	if maxWidrQty <= 0 {
		return
	}

	var price = 1.0

	if asset.Asset != "USDT" {
		syb := coin + "USDT"
		pos, ok := acct.FuPos(syb)
		if ok {
			price, err = fuPrice(pos)
		} else {
			price, err = fuPriceByQuerying(syb)
		}
		if err != nil {
			return
		}
	}

	widrQty := math.Min(maxWidrQty, shouldWidrValue/price)

	// Here must discount widrQty,
	// because price is changing timely.
	// While withdraw request sending to cex,
	// the widrQty is changing with the asset price.
	widrQty = mathy.RoundFloor(widrQty*0.99, 5)

	_, tranRes, errResp := user.Transfer(bnc.TransferTypeUmfutureMain, coin, widrQty)
	if errResp.IsNotNil() {
		err = errResp.Err
		return
	}

	finalWidrValue = widrQty * price
	return
}

//func (m *Main) AdjustHighLtvOrds(user *bnc.User, acct *Account) (results []bnc.CryptoLoanFlexibleLoanAdjustLtvResult, errs []error) {
//	var highLtvOrdNeeds []HighLtvOrdNeed
//
//	for i, ord := range acct.LoanOrders {
//		ltv0 := ord.CurrentLTV
//		if ltv0 <= 0 {
//			continue
//		}
//		var ltv1 float64
//		if quolityCollts[ord.CollateralCoin] {
//			ltv1 = middleQualityCollateralLtv
//		} else {
//			ltv1 = middleSubordinateCollateralLtv
//		}
//		moreColl := ord.CollateralAmount * (ltv0/ltv1 - 1)
//
//		bal, _ := acct.SpotBal(ord.CollateralCoin)
//		lbal, _ := acct.SpotBal("LD" + ord.CollateralCoin)
//
//		allSpot := mathy.RoundFloor((bal.Free+lbal.Free)*0.9999, 5)
//
//		addColl := math.Min(allSpot, moreColl)
//		addColl = mathy.RoundFloor(addColl, 5)
//
//		if addColl < moreColl*0.001 {
//			addColl = 0
//		}
//
//		newLtv0 := ltv0 / (1 + addColl/ord.CollateralAmount)
//
//		usdtNeed := ord.TotalDebt * (1 - ltv1/newLtv0)
//
//		if addColl > 0 {
//			_, adRes, err := user.CryptoLoanFlexibleAdjustLtv(ord.LoanCoin, ord.CollateralCoin, addColl, bnc.LTVAdditional)
//			if err.IsNotNil() {
//				usdtNeed = ord.TotalDebt * (1 - ltv1/ltv0)
//				errs = append(errs, err.Err)
//			} else {
//				results = append(results, adRes)
//			}
//		}
//
//		if i != len(acct.lnOrds)-1 {
//			time.Sleep(time.Second * 2)
//		}
//
//		if usdtNeed/ord.TotalDebt < 0.01 {
//			continue
//		}
//
//		usdtNeed = mathy.RoundFloor(usdtNeed, 2)
//
//		need := HighLtvOrdNeed{ord: ord, usdtNeed: usdtNeed}
//
//		highLtvOrdNeeds = append(highLtvOrdNeeds, need)
//	}
//
//	if len(highLtvOrdNeeds) <= 0 {
//		return
//	}
//
//	sort.Slice(highLtvOrdNeeds, func(i, j int) bool {
//		needI := highLtvOrdNeeds[i]
//		needJ := highLtvOrdNeeds[j]
//		return needI.ord.CurrentLTV > needJ.ord.CurrentLTV
//	})
//
//	var totalUsdtNeed float64
//	for _, need := range highLtvOrdNeeds {
//		totalUsdtNeed += need.usdtNeed
//	}
//
//	usdtSpBal, _ := acct.SpotBal("USDT")
//	usdtSpotRemaining := usdtSpBal.Free - minUsdt
//
//	usdtGap := math.Max(totalUsdtNeed-usdtSpotRemaining, 0)
//
//	if usdtGap > minUsdt {
//		_usdtGap, newErrs := m.BorrowMoreUsdt(user, acct, usdtGap)
//		for _, err := range newErrs {
//			errs = append(errs, err)
//		}
//		usdtGap = _usdtGap
//		// must sleep 2 seconds at least
//		time.Sleep(time.Second * 2)
//	}
//
//	if usdtGap > minUsdt {
//		c.cutPositions(flow, usdtGap, true, true)
//	}
//
//	if c.querySpotAcct(flow).Stopped() {
//		return flow.GetStatus()
//	}
//
//	usdtSpotRemaining = flow.SpBalance("USDT").Free - minUsdt
//
//	for _, need := range highLtvOrdNeeds {
//		if usdtSpotRemaining < minUsdt {
//			break
//		}
//		repayNum := math.Min(need.usdtNeed, usdtSpotRemaining)
//		repayNum = mathy.RoundFloor(repayNum, 2)
//		if repayNum < 1 {
//			continue
//		}
//		flow.Info("Repaying", repayNum, need.ord.LoanCoin, "by", need.ord.CollateralCoin)
//		resp, repayRes, err := c.user.CryptoLoanFlexibleRepay(need.ord.LoanCoin, need.ord.CollateralCoin, repayNum, bnc.BigFalse, bnc.BigFalse)
//		if err.IsNotNil() {
//			return flow.ReqErr(resp, err, 1, "Cannot repay", repayNum, need.ord.LoanCoin, "by", need.ord.CollateralCoin)
//		} else {
//			usdtSpotRemaining -= repayNum
//			flow.Info(repayNum, repayRes.LoanCoin, "repayed")
//		}
//		time.Sleep(time.Second * 2)
//	}
//
//	return flow.GetStatus()
//}
//
//func (m *Main) BorrowMoreUsdt(user *bnc.User, acct *Account, totalBorrow float64) (remainingNeed float64, errs []error) {
//	remainingNeed = totalBorrow
//
//	if totalBorrow < minUsdt {
//		return
//	}
//
//	//if c.queryLoanOngoingOrds(flow).Stopped() {
//	//	return remainingNeed, flow.GetStatus()
//	//}
//	//
//	//if c.querySpotAcct(flow).Stopped() {
//	//	return remainingNeed, flow.GetStatus()
//	//}
//
//	ords := acct.LoanOrders
//
//	_, _colls, errReq := user.CryptoLoanFlexibleCollateralAssets("")
//	colls := map[string]bnc.CryptoLoanFlexibleCollateralCoin{}
//
//	if errReq.IsNotNil() {
//		errs = append(errs, errReq.Err)
//		return
//	} else {
//		for _, coll := range _colls.Rows {
//			colls[coll.CollateralCoin] = coll
//		}
//	}
//
//	for _, ord := range ords {
//		if remainingNeed < minUsdt {
//			break
//		}
//
//		coin := ord.CollateralCoin
//		collInfo, ok := colls[coin]
//		loanCap := 1000_0000.0
//		if ok {
//			loanCap = (collInfo.MaxLimit - ord.TotalDebt) * 0.9
//		}
//
//		bal, _ := acct.SpotBal(coin)
//		lbal, _ := acct.SpotBal("LD" + coin)
//		collNum := bal.Free + lbal.Free
//		if ord.CollateralAmount <= 0 {
//			continue
//		}
//		borrowCap := collNum / ord.CollateralAmount * ord.TotalDebt
//		borrowQty := math.Min(borrowCap, remainingNeed)
//		borrowQty = math.Min(borrowQty, loanCap)
//		borrowQty = mathy.RoundFloor(borrowQty, 2)
//
//		if borrowQty < minUsdt {
//			continue
//		}
//
//		_, res, errReq := user.CryptoLoanFlexibleBorrow(ord.LoanCoin, ord.CollateralCoin, borrowQty, 0)
//		if errReq.IsNotNil() {
//			errs = append(errs, errReq.Err)
//		} else {
//			remainingNeed -= borrowQty
//			m.logger.Info("USDT Borrowed", "result", res)
//		}
//		time.Sleep(time.Second * 2)
//	}
//
//	remainingNeed = math.Max(remainingNeed, 0)
//
//	return
//}
//
//func (m *Main) cutPositions(acct *Account, usdtNeed float64, shouldWidrUsdt, cutFromHighProfitPos bool) (unCutUsdt float64, err error) {
//	unCutUsdt = usdtNeed
//
//	fuUsdtAss, _ := acct.FuAsset("USDT")
//
//	if !shouldWidrUsdt && usdtNeed <= fuUsdtAss.MaxWithdrawAmount {
//		flow.Info(fmt.Sprintf("future USDT max withdraw amount %v > usdt nedd %v", fuUsdtAss.MaxWithdrawAmount, usdtNeed))
//		return unCutUsdt, flow.GetStatus()
//	}
//
//	fuAcct := flow.LatestFuAcct()
//
//	spPairs, flowStatus := c.querySpotPairs(flow)
//	if flowStatus.Stopped() {
//		return
//	}
//	fuPairs, flowStatus := c.queryFuturesPairs(flow)
//	if flowStatus.Stopped() {
//		return
//	}
//
//	poss := fuAcct.Positions
//	sort.Slice(poss, func(i, j int) bool {
//		pi := poss[i]
//		pj := poss[j]
//
//		vi := pi.PositionInitialMargin * pi.Leverage
//		vj := pj.PositionInitialMargin * pj.Leverage
//
//		var ri, rj = -99999999.0, -99999999.0
//		if vi > 0 {
//			ri = pi.UnrealizedProfit / vi
//		}
//
//		if vj > 0 {
//			rj = pj.UnrealizedProfit / pj.PositionInitialMargin * pj.Leverage
//		}
//
//		return (ri > rj) == cutFromHighProfitPos
//	})
//
//	// TODO should consider again
//	if fuUsdtAss.MaxWithdrawAmount > 0 {
//		unCutUsdt -= fuUsdtAss.MaxWithdrawAmount
//	}
//	if fuAcct.TotalUnrealizedProfit < 0 {
//		unCutUsdt -= fuAcct.TotalUnrealizedProfit
//	}
//	if fuUsdtAss.WalletBalance < 0 {
//		unCutUsdt -= fuUsdtAss.WalletBalance
//	}
//
//	for _, pos := range poss {
//		const minRelease = 10.0
//
//		if unCutUsdt < minRelease {
//			break
//		}
//
//		// TODO should confirm that UPNL is position UPNL or not
//		totalPosRelease := pos.UnrealizedProfit + pos.PositionInitialMargin
//		if totalPosRelease < minRelease {
//			continue
//		}
//
//		syb := pos.Symbol
//		sybs := strings.Split(syb, "USDT")
//		if len(sybs) != 2 {
//			continue
//		}
//		coin := sybs[0]
//
//		price := c.priceByPos(pos)
//		if price == 0 {
//			continue
//		}
//
//		posAmt := pos.AbsPositionAmt()
//
//		coinSpQty := flow.SpBalance(coin).Free
//
//		cutQty := math.Min(posAmt, coinSpQty)
//
//		releasePerQty := totalPosRelease/posAmt + price
//
//		cutQty = math.Min(cutQty, unCutUsdt/releasePerQty)
//
//		cutValue := cutQty / posAmt * pos.PositionInitialMargin * pos.Leverage
//
//		// TODO should query from binance
//		if cutValue < minFuTradeUsdt || cutValue < minSpotTradeUsdt {
//			continue
//		}
//
//		spPair, ok := spPairs[syb]
//		if !ok {
//			flow.Err(fmt.Errorf("can not get %v spot pair info", syb), 1, "Cannot find", syb, "spot pair")
//			continue
//		}
//
//		fuPair, ok := fuPairs[syb]
//		if !ok {
//			flow.Err(fmt.Errorf("can not get %v future pair info", syb), 1, "Cannot find", syb, "futures pair")
//			continue
//		}
//
//		isFuCut, spReleaseUsdt, status := c.cutOneTokenPosition(flow, spPair, fuPair, cutQty)
//		if status.Stopped() {
//			flow.SetStatus(FlowWarn)
//		}
//
//		if isFuCut {
//			unCutUsdt -= cutQty * totalPosRelease / posAmt
//		}
//
//		unCutUsdt -= spReleaseUsdt
//
//	}
//
//	flow.Info(false, fmt.Sprintf("remaining usdt is %v", unCutUsdt))
//
//	if !shouldWidrUsdt {
//		return
//	}
//
//	widrUsdt := usdtNeed
//
//	if !c.queryFuAcct(flow).Stopped() {
//		widrUsdt = flow.FuAsset("USDT").MaxWithdrawAmount
//	}
//
//	if widrUsdt < minUsdt {
//		return
//	}
//
//	widrUsdt = mathy.RoundFloor(widrUsdt, 2)
//
//	flow.Info("Transferring", widrUsdt, "USDT from futures to spot")
//	resp, tranRes, errReq := c.user.Transfer(bnc.TransferTypeUmfutureMain, "USDT", widrUsdt)
//	if errReq.IsNotNil() {
//		return unCutUsdt, flow.ReqErr(resp, errReq, 1, "Cannot transfer", widrUsdt, "USDT from futures to spot")
//	}
//
//	flow.Info(widrUsdt, "USDT transferred from futures to spot,", tranRes)
//	return unCutUsdt, flow.GetStatus()
//}
//
//func (m *Main) cutOneTokenPosition(flow *Flow, spPair, fuPair cex.Pair, qty float64) (isFuCut bool, spUsdt float64, flowStatus FlowStatus) {
//	qPrec := int32(math.Min(float64(spPair.QPrecision), float64(fuPair.QPrecision)))
//
//	qty = mathy.RoundFloor(qty, qPrec)
//
//	flow.Info("Buying", qty, fuPair.Asset)
//	resp, fuOpenOrd, err := c.user.NewFuturesMarketBuyOrder(fuPair.Asset, fuPair.Quote, qty)
//	if err.IsNotNil() {
//		return isFuCut, spUsdt, flow.ReqErr(resp, err, 1, "Cannot new", qty, fuPair.Asset, "futures market order")
//	}
//
//	flow.AddOrder(*fuOpenOrd)
//
//	ctxFuWait, cancelFuWait := context.WithCancel(context.Background())
//	defer cancelFuWait()
//	chErr := c.user.WaitOrder(ctxFuWait, fuOpenOrd)
//	select {
//	case err = <-chErr:
//	case <-time.After(time.Second * 10):
//	}
//
//	flow.AddOrder(*fuOpenOrd)
//
//	if err.IsNotNil() || ctxFuWait.Err() != nil {
//		flow.Err(fmt.Errorf("reqerr: %w, ctxerr: %w", err.Err, ctxFuWait.Err()), 1, "Cannot wait futures order")
//		return isFuCut, spUsdt, flow.GetStatus()
//	}
//
//	isFuCut = true
//
//	if c.queryFuAcct(flow).Stopped() {
//		return isFuCut, spUsdt, flow.GetStatus()
//	}
//
//	coinSpotBal := flow.SpBalance(spPair.Asset)
//
//	_ = c.querySpotAcct(flow)
//
//	coinSpotBal = flow.SpBalance(spPair.Asset)
//
//	coinSpotQty := mathy.RoundFloor(math.Min(coinSpotBal.Free, qty), int32(spPair.QPrecision))
//
//	if coinSpotQty < qty*0.9999 {
//		flow.Disaster(fmt.Errorf("%v spot balance is %v, smaller than future close qty %v", spPair.Asset, coinSpotQty, qty), 1, "Spot account has no more coins!")
//		return isFuCut, spUsdt, flow.GetStatus()
//	}
//
//	spotResp, spotOrder, err := c.user.NewSpotMarketSellOrder(spPair.Asset, spPair.Quote, coinSpotQty, cex.CltOptRetryCount(3, time.Second*2))
//	if err.IsNotNil() {
//		return isFuCut, spUsdt, flow.ReqErr(spotResp, err, 1, "Cannot new market order")
//	}
//	flow.AddOrder(*spotOrder)
//
//	ctxSpWait, cancelSpWait := context.WithCancel(context.Background())
//	defer cancelSpWait()
//	chErr = c.user.WaitOrder(ctxSpWait, spotOrder)
//	select {
//	case err = <-chErr:
//	case <-time.After(time.Second * 10):
//	}
//	flow.AddOrder(*spotOrder)
//	if err.IsNotNil() || ctxFuWait.Err() != nil {
//		flow.Err(fmt.Errorf("waiterr: %w, ctxerr: %w", err.Err, ctxFuWait.Err()), 1, "Cannot wait spot order")
//		return
//	}
//
//	spUsdt = spotOrder.FilledQuote
//
//	return
//}

func (m *Main) NewPosSlowly(spPair, fuPair cex.Pair, spSide cex.OrderSide, spQty, fuExp float64, times int64) error {
	everyTime := mathy.RoundFloor(spQty/float64(times), int32(spPair.QPrecision))
	m.logger.Info("Every Position Qty", "qty", everyTime)
	for i := int64(0); i < times; i++ {
		//_, acct, err := m.acctWatcher.Update()
		//if err != nil {
		//	return err
		//}
		//bal, _ := acct.SpotBal("USDT")
		//if bal.Free < 1000 {
		//	break
		//}
		err := m.NewPos(spPair, fuPair, spSide, everyTime, fuExp)
		if err != nil {
			return err
		}
		time.Sleep(time.Second)
	}
	return nil
}

// NewPos
// fuQty = spQty / fuExp
func (m *Main) NewPos(spPair, fuPair cex.Pair, spSide cex.OrderSide, spQty, fuExp float64) error {
	m.logger.Info("New Position")

	if fuExp == 0 {
		m.logger.Error("fuExp Is 0")
		return errors.New("fu exp is 0")
	}

	spQty = mathy.RoundFloor(spQty, int32(spPair.QPrecision))

	fuQty := spQty / fuExp
	fuQty = mathy.RoundFloor(fuQty, int32(fuPair.QPrecision))
	logger := m.logger.With("spotSide", spSide, "spotPair", spPair.PairSymbol, "fuPair", fuPair.PairSymbol, "spQty", spQty, "fuQty", fuQty)

	if spQty <= spPair.MinTradeQty {
		m.logger.Error("Spot Qty Is Too Little", "qty", spQty)
		return errors.New("spot qty is too little")
	}

	if fuQty <= fuPair.MinTradeQty {
		m.logger.Error("Futures Qty Is Too Little", "qty", fuQty)
		return errors.New("futures qty is too little")
	}

	var spTrader, fuTrader cex.MarketTraderFunc
	switch spSide {
	case cex.OrderSideBuy:
		spTrader = m.user.NewSpotMarketBuyOrder
		fuTrader = m.user.NewFuturesMarketBuyOrder
	case cex.OrderSideSell:
		spTrader = m.user.NewSpotMarketSellOrder
		fuTrader = m.user.NewFuturesMarketSellOrder
	}

	logger.Info("Placing Spot Market Order")

	_, spOrd, reqErr := spTrader(spPair.Asset, spPair.Quote, spQty)
	if reqErr.IsNotNil() {
		logger.Error("Cannot Place Spot Market Order", "err", reqErr.Err)
		return reqErr.Err
	}

	logger.Info("Waiting Spot Market Order")

	chReqErr := m.user.WaitOrder(context.Background(), spOrd)
	reqErr = <-chReqErr
	if reqErr.IsNotNil() {
		logger.Error("Cannot Wait Spot Market Order", "err", reqErr.Err)
		return reqErr.Err
	}

	if !spOrd.IsFinished() {
		logger.Error("Spot Market Order Is Not Finished", "err", reqErr.Err)
		return errors.New("spot market order is not finished")
	}

	logger.Info("Spot Market Order Is Finished", "filledQty", spOrd.FilledQty, "filledAvgPrice", spOrd.FilledAvgPrice)

	for {
		logger.Info("New Futures Market Order")
		_, fuOrd, reqErr := fuTrader(fuPair.Asset, fuPair.Quote, fuQty)
		if reqErr.IsNotNil() {
			logger.Error("Cannot Trade Futures", "err", reqErr.Err)
			time.Sleep(time.Second * 2)
			continue
		}

		logger.Info("Waiting Futures Order")

		chReqErr := m.user.WaitOrder(context.Background(), fuOrd)
		reqErr = <-chReqErr
		if reqErr.IsNotNil() {
			logger.Error("Cannot Wait Futures Market Order", "err", reqErr.Err)
			time.Sleep(time.Second * 2)
			continue
		}

		if spOrd.IsFinished() {
			logger.Info("Futures Market Order Is Finished")
			break
		} else {
			logger.Error("Futures Market Order Is Not Finished")
			return errors.New("futures market order is not finished")
		}
	}

	return nil
}

func NewHUANGYANMain() {
	apiKeys, err := cex.ReadApiKey()
	props.PanicIfNotNil(err)
	key, ok := apiKeys["HUANGYAN"]
	if !ok {
		panic(0)
	}
	user := bnc.NewUser(key.ApiKey, key.SecretKey, bnc.UserOptSetPositionBothSide(), bnc.UserOptSetPortfolioMarginAccount())
	m, err := NewMain(user, nil)
	props.PanicIfNotNil(err)
	spPairs, err := QuerySpotPairs()
	props.PanicIfNotNil(err)
	fuPairs, err := QueryFuPairs()
	props.PanicIfNotNil(err)
	spSyb := "ETHUSDT"
	fuSyb := spSyb
	fuExp := 1.0
	spPair, ok := spPairs[spSyb]
	if !ok {
		panic("no spot pair")
	}
	if !spPair.Tradable {
		panic("spot is not tradable")
	}
	fuPair, ok := fuPairs[fuSyb]
	if !ok {
		panic("no futures pair")
	}
	if !fuPair.Tradable {
		panic("futures is not tradable")
	}
	err = m.NewPosSlowly(spPair, fuPair, cex.OrderSideBuy, 1, fuExp, 1)
	props.PanicIfNotNil(err)
}
