package frbnc

import (
	"context"
	"sync"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
)

type VIPPortmarPosStatus string
type VIPPortmarOrderStatus string

const (
	VIPPortmarPosStatusNone             VIPPortmarPosStatus = ""
	VIPPortmarPosStatusFuOpening        VIPPortmarPosStatus = "FU_OPENING"
	VIPPortmarPosStatusSpOpening        VIPPortmarPosStatus = "SP_OPENING"
	VIPPortmarPosStatusReFuOpening      VIPPortmarPosStatus = "RE_FU_OPENING"
	VIPPortmarPosStatusFuOpened         VIPPortmarPosStatus = "FU_OPENED"
	VIPPortmarPosStatusSpOpened         VIPPortmarPosStatus = "SP_OPENED"
	VIPPortmarPosStatusReFuOpened       VIPPortmarPosStatus = "RE_FU_OPENED"
	VIPPortmarPosStatusFuFailed         VIPPortmarPosStatus = "FU_FAILED"
	VIPPortmarPosStatusSpFailed         VIPPortmarPosStatus = "SP_FAILED"
	VIPPortmarPosStatusReFuFailed       VIPPortmarPosStatus = "RE_FU_FAILED"
	VIPPortmarPosStatusFuWaiterFailed   VIPPortmarPosStatus = "FU_WAITER_FAILED"
	VIPPortmarPosStatusSpWaiterFailed   VIPPortmarPosStatus = "SP_WAITER_FAILED"
	VIPPortmarPosStatusReFuWaiterFailed VIPPortmarPosStatus = "RE_FU_WAITER_FAILED"

	VIPPortmarOrderStatusNone         VIPPortmarOrderStatus = ""
	VIPPortmarOrderStatusOpening      VIPPortmarOrderStatus = "OPENING"
	VIPPortmarOrderStatusOpened       VIPPortmarOrderStatus = "OPENED"
	VIPPortmarOrderStatusFailed       VIPPortmarOrderStatus = "FAILED"
	VIPPortmarOrderStatusWaiterFailed VIPPortmarOrderStatus = "WAITER_FAILED"
)

type VIPPortmarOrd struct {
	Order  cex.Order
	Status VIPPortmarOrderStatus
	Err    error
}

type VIPPortmarPosMsg struct {
	FuOrd   VIPPortmarOrd
	SpOrd   VIPPortmarOrd
	ReFuOrd VIPPortmarOrd
	Errs    []error
}

func (v VIPPortmarPosMsg) Status() VIPPortmarPosStatus {
	switch v.ReFuOrd.Status {
	case VIPPortmarOrderStatusOpened:
		return VIPPortmarPosStatusReFuOpened
	case VIPPortmarOrderStatusOpening:
		return VIPPortmarPosStatusReFuOpening
	case VIPPortmarOrderStatusFailed:
		return VIPPortmarPosStatusReFuFailed
	case VIPPortmarOrderStatusWaiterFailed:
		return VIPPortmarPosStatusReFuWaiterFailed
	}

	switch v.SpOrd.Status {
	case VIPPortmarOrderStatusOpened:
		return VIPPortmarPosStatusSpOpened
	case VIPPortmarOrderStatusOpening:
		return VIPPortmarPosStatusSpOpening
	case VIPPortmarOrderStatusFailed:
		return VIPPortmarPosStatusSpFailed
	case VIPPortmarOrderStatusWaiterFailed:
		return VIPPortmarPosStatusSpWaiterFailed
	}

	switch v.FuOrd.Status {
	case VIPPortmarOrderStatusOpening:
		return VIPPortmarPosStatusFuOpening
	case VIPPortmarOrderStatusOpened:
		return VIPPortmarPosStatusFuOpened
	case VIPPortmarOrderStatusFailed:
		return VIPPortmarPosStatusFuFailed
	case VIPPortmarOrderStatusWaiterFailed:
		return VIPPortmarPosStatusFuWaiterFailed
	}
	return VIPPortmarPosStatusNone
}

type VIPPortmarPosMsger struct {
	mux       sync.RWMutex
	latestMsg VIPPortmarPosMsg
	chMsg     chan VIPPortmarPosMsg
}

func (v *VIPPortmarPosMsger) SendMsg(msg VIPPortmarPosMsg) {
	v.mux.Lock()
	defer v.mux.Unlock()
	v.latestMsg = msg
	v.chMsg <- msg
}

func (v *VIPPortmarPosMsger) GetLatestMsg() VIPPortmarPosMsg {
	v.mux.RLock()
	defer v.mux.RUnlock()
	return v.latestMsg
}

func (v *VIPPortmarPosMsger) Status() VIPPortmarPosStatus {
	v.mux.RLock()
	defer v.mux.RUnlock()
	return v.latestMsg.Status()
}

func VIPPortmarMarketTraderFunc(ctx context.Context, user *bnc.User, pair cex.Pair, trader cex.MarketTraderFunc, qty float64) (ord VIPPortmarOrd, err error) {
	ord.Status = VIPPortmarOrderStatusOpening

	_, oriOrd, reqErr := trader(pair.Asset, pair.Quote, qty)

	if oriOrd != nil {
		ord.Order = *oriOrd
	}

	if reqErr.IsNotNil() {
		ord.Status = VIPPortmarOrderStatusFailed
		ord.Err = reqErr.Err
		err = reqErr.Err
		return
	}

	reqErr = <-user.WaitOrder(ctx, oriOrd)

	if reqErr.IsNotNil() {
		ord.Status = VIPPortmarOrderStatusWaiterFailed
		ord.Err = reqErr.Err
		err = reqErr.Err
		return
	}

	if oriOrd != nil {
		ord.Order = *oriOrd
	}

	ord.Status = VIPPortmarOrderStatusOpened

	return
}

type VIPPortmarPosTraderParams struct {
	User   *bnc.User
	SpSide cex.OrderSide
	IsCM   bool
	SpPair cex.Pair
	SpQty  float64
	FuPair cex.Pair
	FuQty  float64
}

func VIPPortmarPosTrader(ctx context.Context, params VIPPortmarPosTraderParams) *VIPPortmarPosMsger {

	var spFunc, fuFunc, reFuFunc cex.MarketTraderFunc

	if params.SpSide == cex.OrderSideBuy {
		spFunc = params.User.NewSpotMarketBuyOrder
		if params.IsCM {
			fuFunc = params.User.NewFuturesMarketSellCMOrder
			reFuFunc = params.User.NewFuturesMarketBuyCMOrder
		} else {
			fuFunc = params.User.NewFuturesMarketSellOrder
			reFuFunc = params.User.NewFuturesMarketBuyOrder
		}
	} else {
		spFunc = params.User.NewSpotMarketSellOrder
		if params.IsCM {
			fuFunc = params.User.NewFuturesMarketBuyCMOrder
			reFuFunc = params.User.NewFuturesMarketSellCMOrder
		} else {
			fuFunc = params.User.NewFuturesMarketBuyOrder
			reFuFunc = params.User.NewFuturesMarketSellOrder
		}
	}

	msger := &VIPPortmarPosMsger{
		chMsg: make(chan VIPPortmarPosMsg, 6),
	}

	go func() {
		var msg VIPPortmarPosMsg

		// must trade futures firstly

		msg.FuOrd.Status = VIPPortmarOrderStatusOpening
		msger.SendMsg(msg)

		fuOrd, err := VIPPortmarMarketTraderFunc(ctx, params.User, params.FuPair, fuFunc, params.FuQty)

		msg.FuOrd = fuOrd

		if err != nil {
			msg.Errs = append(msg.Errs, err)
			msger.SendMsg(msg)
			return
		}

		msger.SendMsg(msg)

		// trade spot

		msg.SpOrd.Status = VIPPortmarOrderStatusOpening
		msger.SendMsg(msg)

		spOrd, err := VIPPortmarMarketTraderFunc(ctx, params.User, params.SpPair, spFunc, params.SpQty)

		msg.SpOrd = spOrd

		if err != nil {
			msg.Errs = append(msg.Errs, err)
			msger.SendMsg(msg)
			if spOrd.Status == VIPPortmarOrderStatusWaiterFailed {
				return
			}

			// reverse futures

			reFuOrd, err := VIPPortmarMarketTraderFunc(ctx, params.User, params.FuPair, reFuFunc, params.FuQty)
			msg.ReFuOrd = reFuOrd
			if err != nil {
				msg.Errs = append(msg.Errs, err)
				msger.SendMsg(msg)
				return
			}
			msger.SendMsg(msg)
			return
		}

		msger.SendMsg(msg)
	}()

	return msger
}
