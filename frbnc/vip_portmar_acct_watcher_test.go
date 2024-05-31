package frbnc

import (
	"testing"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/dwdwow/props"
)

func TestNewVIPPortmarAcctWatcher(t *testing.T) {
	keys, err := cex.ReadApiKey()
	props.PanicIfNotNil(err)
	key, ok := keys["HUANGYAN"]
	if !ok {
		panic("not ok")
	}
	watcher := NewVIPPortmarAcctWatcher(bnc.NewUser(key.ApiKey, key.SecretKey), nil)
	props.PanicIfNotNil(watcher.Start())

	for e := range watcher.Sub() {
		props.PrintlnIndent(e)
	}
}
