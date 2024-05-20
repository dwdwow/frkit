package frbnc

import "github.com/dwdwow/cex/bnc"

type VIPPortmarAccount struct {
	ApiKey string          `json:"apiKey"`
	Time   int64           `json:"time"`
	Spot   bnc.SpotAccount `json:"spot"`

	PortmarAccountDetail      bnc.PortfolioMarginAccountDetail      `json:"portmarAccountDetail"`
	PortmarAccountInformation bnc.PortfolioMarginAccountInformation `json:"portmarAccountInformation"`

	LoanOrders     []bnc.VIPLoanOngoingOrder        `json:"loanOrders"`
	LoanStatusInfo bnc.VIPLoanApplicationStatusInfo `json:"loanStatusInfo"`
}
