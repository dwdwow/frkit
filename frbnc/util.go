package frbnc

import (
	"fmt"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
)

func slice2map[T any, S []T](s S, key func(T) string) map[string]T {
	m := map[string]T{}
	for _, t := range s {
		m[key(t)] = t
	}
	return m
}

func fuPrice(pos bnc.FuturesAccountPosition) (price float64, err error) {
	posAmt := pos.AbsPositionAmt()
	if posAmt > 0 {
		price = pos.PositionInitialMargin * pos.Leverage / posAmt
		return
	}
	return fuPriceByQuerying(pos.Symbol)
}

func fuPriceByQuerying(symbol string) (price float64, err error) {
	if symbol == "USDTUSDT" {
		return 1, nil
	}

	rawOb, err := bnc.QueryFuturesOrderBook(symbol, 100)
	if err != nil {
		return
	}

	bids := rawOb.Bids
	if len(bids) <= 0 {
		return price, fmt.Errorf("%v orderbook bids len <= 0", symbol)
	}
	bid0 := bids[0]
	if len(bid0) != 2 {
		return price, fmt.Errorf("%v orderbook bid0 len %v != 2", symbol, len(bid0))
	}
	price = bid0[0]
	if price <= 0 {
		return price, fmt.Errorf("%v price %v <= 0", symbol, price)
	}

	return
}

func queryPairs(f func() ([]cex.Pair, bnc.ExchangeInfo, error)) (map[string]cex.Pair, error) {
	_spPairs, _, err := f()
	if err != nil {
		return nil, err
	}
	spPairs := map[string]cex.Pair{}
	for _, pair := range _spPairs {
		spPairs[pair.PairSymbol] = pair
	}
	return spPairs, nil
}

func QuerySpotPairs() (map[string]cex.Pair, error) {
	return queryPairs(bnc.QuerySpotPairs)
}

func QueryFuPairs() (map[string]cex.Pair, error) {
	return queryPairs(bnc.QueryFuturesPairs)
}
