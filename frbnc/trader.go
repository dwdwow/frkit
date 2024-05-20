package frbnc

import (
	"log/slog"
	"os"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/dwdwow/props"
)

type SpFuTraderParams struct {
	SpPair cex.Pair
	FuPair cex.Pair
	SpSide cex.OrderSide
	FuSide cex.OrderSide
	SpQty  float64
	FuQty  float64
}

type SpFuOrderStatus string

const (
	SpFuTraderStatusNone     SpFuOrderStatus = ""
	SpFuTraderStatusOpening  SpFuOrderStatus = "OPENING"
	SpFuTraderStatusFailed   SpFuOrderStatus = "FAILED"
	SpFuTraderStatusReFailed SpFuOrderStatus = "RE_FAILED"
	SpFuTraderStatusFilled   SpFuOrderStatus = "FILLED"
)

type SpFuTrader struct {
	spStatus props.SafeRWData[SpFuOrderStatus]
	fuStatus props.SafeRWData[SpFuOrderStatus]

	user *bnc.User

	Params SpFuTraderParams

	SpOrders props.SafeRWSlice[*cex.Order]
	FuOrders props.SafeRWSlice[*cex.Order]

	logger *slog.Logger
}

func NewSpFuTrader(user *bnc.User, params SpFuTraderParams, logger *slog.Logger) *SpFuTrader {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	return &SpFuTrader{
		user:   user,
		Params: params,
		logger: logger,
	}
}

//func (t *SpFuTrader) trade(tradeFunc cex.MarketTraderFunc, asset, quote string, qty float64) cex.RequestError {
//	chNewOrd := make(chan *cex.Order)
//	chNewOrdErr := make(chan cex.RequestError)
//
//	go func() {
//		_, ord, err := tradeFunc(asset, quote, qty, cex.CltOptRetryCount(3, time.Second))
//		if err.IsNotNil() {
//			chNewOrdErr <- err
//			return
//		}
//		chNewOrd <- ord
//	}()
//
//	var ord *cex.Order
//	waitSeconds := int64(5)
//
//	for {
//		select {
//		case <-time.After(time.Duration(waitSeconds) * time.Second):
//			t.logger.Error("New Market Order Is Timeout", "waitSeconds", waitSeconds)
//		case err := <-chNewOrdErr:
//			if err.IsNotNil() {
//				return err
//			}
//		case ord = <-chNewOrd:
//		}
//		if ord != nil {
//			break
//		}
//	}
//
//	t.user.WaitOrder(context.Background(), ord)
//}
