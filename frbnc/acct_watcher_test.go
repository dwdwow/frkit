package frbnc

import (
	"testing"

	"github.com/dwdwow/cex"
	"github.com/dwdwow/cex/bnc"
	"github.com/dwdwow/props"
)

func TestNewAcctWatcher(t *testing.T) {
	keys, err := cex.ReadApiKey()
	props.PanicIfNotNil(err)
	key, ok := keys["TEST"]
	if !ok {
		panic("not ok")
	}
	watcher := NewAcctWatcher(bnc.NewUser(key.ApiKey, key.SecretKey), nil)
	err = watcher.Start()
	props.PanicIfNotNil(err)
	c := watcher.Sub()
	for {
		props.PrintlnIndent(<-c)
	}
}
