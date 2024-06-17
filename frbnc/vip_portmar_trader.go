package frbnc

import (
	"context"
	"sync"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
)

type VIPPortmarPosStatus string

const (
	VIPPortmarPosStatusNone        VIPPortmarPosStatus = "NONE"
	VIPPortmarPosStatusNew         VIPPortmarPosStatus = "NEW"
	VIPPortmarPosStatusFuOpening   VIPPortmarPosStatus = "FU_OPENING"
	VIPPortmarPosStatusSpOpening   VIPPortmarPosStatus = "SP_OPENING"
	VIPPortmarPosStatusReFuOpening VIPPortmarPosStatus = "RE_FU_OPENING"
	VIPPortmarPosStatusFuOpened    VIPPortmarPosStatus = "FU_OPENED"
	VIPPortmarPosStatusSpOpened    VIPPortmarPosStatus = "SP_OPENED"
	VIPPortmarPosStatusReFuOpened  VIPPortmarPosStatus = "RE_FU_OPENED"
	VIPPortmarPosStatusFuFailed    VIPPortmarPosStatus = "FU_FAILED"
	VIPPortmarPosStatusSpFailed    VIPPortmarPosStatus = "SP_FAILED"
	VIPPortmarPosStatusReFuFailed  VIPPortmarPosStatus = "RE_FU_FAILED"
)

type VIPPortmarPosMsg struct {
	Status  VIPPortmarPosStatus
	FuOrd   cex.Order
	SpOrd   cex.Order
	ReFuOrd cex.Order
	Err     error
}

type VIPPortmarPosMsger struct {
	mux       sync.RWMutex
	Status    VIPPortmarPosStatus
	LatestMsg VIPPortmarPosMsg
	MsgChan   chan VIPPortmarPosMsg
}

func (v *VIPPortmarPosMsger) SetStatus(status VIPPortmarPosStatus) {
	v.mux.Lock()
	v.Status = status
	v.mux.Unlock()
}

func (v *VIPPortmarPosMsger) GetStatus() VIPPortmarPosStatus {
	v.mux.RLock()
	defer v.mux.RUnlock()
	return v.Status
}

func (v *VIPPortmarPosMsger) SetLatestMsg(msg VIPPortmarPosMsg) {
	v.mux.Lock()
	v.LatestMsg = msg
	v.mux.Unlock()
}

func (v *VIPPortmarPosMsger) GetLatestMsg() VIPPortmarPosMsg {
	v.mux.RLock()
	defer v.mux.RUnlock()
	return v.LatestMsg
}

func VIPPortmarPosTrader(ctx context.Context, user *bnc.User, spSide cex.OrderSide, isCM bool, spPair cex.Pair, spQty float64, fuPair cex.Pair, fuQty float64) *VIPPortmarPosMsger {

	var spFunc, fuFunc, reFuFunc cex.MarketTraderFunc

	if spSide == cex.OrderSideBuy {
		spFunc = user.NewSpotMarketBuyOrder
		if isCM {
			fuFunc = user.NewFuturesMarketSellCMOrder
			reFuFunc = user.NewFuturesMarketBuyCMOrder
		} else {
			fuFunc = user.NewFuturesMarketSellOrder
			reFuFunc = user.NewFuturesMarketBuyOrder
		}
	} else {
		spFunc = user.NewSpotMarketSellOrder
		if isCM {
			fuFunc = user.NewFuturesMarketBuyCMOrder
			reFuFunc = user.NewFuturesMarketSellCMOrder
		} else {
			fuFunc = user.NewFuturesMarketBuyOrder
			reFuFunc = user.NewFuturesMarketSellOrder
		}
	}

	msger := &VIPPortmarPosMsger{
		Status:  VIPPortmarPosStatusNew,
		MsgChan: make(chan VIPPortmarPosMsg, 3),
	}

	go func() {
		// must trade future firstly

		msger.SetStatus(VIPPortmarPosStatusFuOpening)

		_, fuOrd, reqErr := fuFunc(fuPair.Asset, fuPair.Quote, fuQty)

		var msg VIPPortmarPosMsg

		if fuOrd != nil {
			msg.FuOrd = *fuOrd
		}

		if reqErr.IsNotNil() {
			msg.Status = VIPPortmarPosStatusFuFailed
			msg.Err = reqErr.Err
			msger.SetStatus(VIPPortmarPosStatusFuFailed)
			msger.SetLatestMsg(msg)
			msger.MsgChan <- msg
			return
		}

		chReqErr := user.WaitOrder(ctx, fuOrd)

		reqErr = <-chReqErr

		msg.FuOrd = *fuOrd

		if reqErr.IsNotNil() {
			msg.Status = VIPPortmarPosStatusFuFailed
			msg.Err = reqErr.Err
			msger.SetStatus(VIPPortmarPosStatusFuFailed)
			msger.SetLatestMsg(msg)
			msger.MsgChan <- msg
			return
		}

		msg.Status = VIPPortmarPosStatusFuOpened
		msg.Err = reqErr.Err
		msger.SetStatus(VIPPortmarPosStatusFuOpened)
		msger.SetLatestMsg(msg)
		msger.MsgChan <- msg

		// trade spot

		msger.SetStatus(VIPPortmarPosStatusSpOpening)

		_, spOrd, reqErr := spFunc(spPair.Asset, spPair.Quote, spQty)

		if spOrd != nil {
			msg.SpOrd = *spOrd
		}

		if reqErr.IsNotNil() {
			msg.Status = VIPPortmarPosStatusSpFailed
			msg.Err = reqErr.Err
			msger.SetStatus(VIPPortmarPosStatusSpFailed)
			msger.SetLatestMsg(msg)
			msger.MsgChan <- msg
			return
		}

		chReqErr = user.WaitOrder(ctx, spOrd)

		reqErr = <-chReqErr

		msg.SpOrd = *spOrd

		if reqErr.IsNotNil() {
			msg.Status = VIPPortmarPosStatusSpFailed
			msg.Err = reqErr.Err
			msger.SetStatus(VIPPortmarPosStatusSpFailed)
			msger.SetLatestMsg(msg)
			msger.MsgChan <- msg

			// reverse future position

			msger.SetStatus(VIPPortmarPosStatusReFuOpening)

			_, reFuOrd, reqErr := reFuFunc(fuPair.Asset, fuPair.Quote, fuQty)

			if reFuOrd != nil {
				msg.ReFuOrd = *reFuOrd
			}

			if reqErr.IsNotNil() {
				msg.Status = VIPPortmarPosStatusReFuFailed
				msg.Err = reqErr.Err
				msger.SetStatus(VIPPortmarPosStatusReFuFailed)
				msger.SetLatestMsg(msg)
				msger.MsgChan <- msg
				return
			}

			chReqErr = user.WaitOrder(ctx, reFuOrd)

			reqErr = <-chReqErr

			msg.ReFuOrd = *reFuOrd

			if reqErr.IsNotNil() {
				msg.Status = VIPPortmarPosStatusReFuFailed
				msg.Err = reqErr.Err
				msger.SetStatus(VIPPortmarPosStatusReFuFailed)
				msger.SetLatestMsg(msg)
				msger.MsgChan <- msg
				return
			}

			msg.Status = VIPPortmarPosStatusReFuOpened
			msg.Err = reqErr.Err
			msger.SetStatus(VIPPortmarPosStatusReFuOpened)
			msger.SetLatestMsg(msg)
			msger.MsgChan <- msg

			return
		}

		msg.Status = VIPPortmarPosStatusSpOpened
		msg.Err = reqErr.Err
		msger.SetStatus(VIPPortmarPosStatusSpOpened)
		msger.SetLatestMsg(msg)
		msger.MsgChan <- msg

	}()

	return msger
}
