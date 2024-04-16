package frbnc

import "github.com/go-resty/resty/v2"

type typeUsdChecker map[string]struct{}

func (u typeUsdChecker) isUsd(coin string) bool {
	_, ok := u[coin]
	return ok
}

var usdChecker typeUsdChecker = map[string]struct{}{
	"USDT": {},
	"USDC": {},
}

var qualityCollts = map[string]bool{
	"BTC": true,
	"ETH": true,
}

const (
	minQualityCollateralLtv    = 0.55
	middleQualityCollateralLtv = 0.6
	maxQualityCollateralLtv    = 0.65

	//minSubordinateCollateralLtv    = 0.4
	//middleSubordinateCollateralLtv = 0.45
	//maxSubordinateCollateralLtv    = 0.5

	minSubordinateCollateralLtv    = 0.55
	middleSubordinateCollateralLtv = 0.6
	maxSubordinateCollateralLtv    = 0.65

	// ratio = totalMarginBalance / totalPositionValue
	minFuturesAccountMarginRatio    = 0.25
	middleFuturesAccountMarginRatio = 0.3
	maxFuturesAccountMarginRatio    = 0.35
)

const (
	minUsdt          = 5.0
	minSpotTradeUsdt = 11.0
	minFuTradeUsdt   = 21.0
)

type MarginCoin struct {
	Coin        string
	PledgeRatio float64
	MaxNum      float64
}

var validMarginCoins = []MarginCoin{
	{"BTC", 0.95, 10},
	{"ETH", 0.95, 100},
	{"BNB", 0.95, 500},
}

type Responses []*resty.Response
