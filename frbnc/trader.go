package frbnc

import (
	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
)

type SpFuTraderParams struct {
	SpPair cex.Pair
	FuPair cex.Pair
	SpSide cex.OrderSide
	FuSide cex.OrderSide
	SpQty  float64
	FuQty  float64
}

type SpFuTraderStatus string

const ()

type SpFuTrader struct {
	status SpFuTraderStatus

	user *bnc.User

	Params SpFuTraderParams

	SpOrders []*cex.Order
	FuOrders []*cex.Order
}

func NewSpFuTrader(user *bnc.User, params SpFuTraderParams) *SpFuTrader {
	return &SpFuTrader{
		user:   user,
		Params: params,
	}
}

func (t *SpFuTrader) trade() {

}
