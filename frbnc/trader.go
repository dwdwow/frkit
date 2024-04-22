package frbnc

import "github.com/dwdwow/cex"

type TraderParams struct {
	SpPair cex.Pair
	FuPair cex.Pair
	SpSide cex.OrderSide
	FuSide cex.OrderSide
	SpQty  float64
	FuQty  float64
}
