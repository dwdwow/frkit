package frbnc

import "testing"

func TestIsQuoteAsset(t *testing.T) {
	type args struct {
		symbol string
		quote  string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"All Empty", args{"", ""}, false},
		{"Same", args{"USDT", "USDT"}, false},
		{"Empty Symbol", args{"", "USDT"}, false},
		{"Quote Is Prefix", args{"USDT", "USD"}, false},
		{"Quote Is Longer", args{"USD", "USDT"}, false},
		{"Right Example", args{"ETHUSDT", "USDT"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsQuoteAsset(tt.args.symbol, tt.args.quote); got != tt.want {
				t.Errorf("IsQuoteAsset() = %v, want %v", got, tt.want)
			}
		})
	}
}
