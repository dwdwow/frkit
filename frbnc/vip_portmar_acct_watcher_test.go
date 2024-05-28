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
	err = watcher.Start()
	props.PanicIfNotNil(err)
	c := watcher.Sub()
	for {
		props.PrintlnIndent(<-c)
	}
}
