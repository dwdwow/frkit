package frbnc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/dwdwow/cex/bnc"
)

type VIPPortmarAcctWatcherMsg struct {
	Acct *VIPPortmarAccount
	Err  error
}

type VIPPortmarAcctWatcher struct {
	user *bnc.User

	ctx       context.Context
	ctxCancel context.CancelFunc

	muxAcct sync.Mutex
	acct    *VIPPortmarAccount

	muxSubbers sync.Mutex
	subbers    []chan VIPPortmarAcctWatcherMsg

	muxClosed sync.Mutex
	closed    bool

	logger *slog.Logger
}

func NewVIPPortmarAcctWatcher(user *bnc.User, logger *slog.Logger) *VIPPortmarAcctWatcher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	logger = logger.With("watcher", user.Api().Cex+"_acct")
	ctx, cancel := context.WithCancel(context.Background())
	return &VIPPortmarAcctWatcher{
		user:      user,
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
	}
}

func (aw *VIPPortmarAcctWatcher) update() (acct *VIPPortmarAccount, err error) {
	aw.logger.Info("Updating Account")
	_, acct, respErr := QueryVIPPortmarAccount(aw.user)
	if respErr.IsNotNil() {
		return nil, respErr.Err
	}
	aw.acct = acct
	return acct, nil
}

func (aw *VIPPortmarAcctWatcher) Update() (updating bool, acct *VIPPortmarAccount, err error) {
	if !aw.muxAcct.TryLock() {
		return true, nil, nil
	}
	defer aw.muxAcct.Unlock()
	acct, err = aw.update()
	return
}

func (aw *VIPPortmarAcctWatcher) Acct() (updating bool, acct *VIPPortmarAccount) {
	if !aw.muxAcct.TryLock() {
		return true, nil
	}
	defer aw.muxAcct.Unlock()
	return false, aw.acct
}

func (aw *VIPPortmarAcctWatcher) addSuber() chan VIPPortmarAcctWatcherMsg {
	aw.muxSubbers.Lock()
	defer aw.muxSubbers.Unlock()
	c := make(chan VIPPortmarAcctWatcherMsg)
	aw.subbers = append(aw.subbers, c)
	return c
}

func (aw *VIPPortmarAcctWatcher) remove(c chan VIPPortmarAcctWatcherMsg) {
	aw.muxSubbers.Lock()
	defer aw.muxSubbers.Unlock()
	for i, suber := range aw.subbers {
		if suber == c {
			aw.subbers = slices.Delete(aw.subbers, i, i)
			return
		}
	}
}

func (aw *VIPPortmarAcctWatcher) Sub() chan VIPPortmarAcctWatcherMsg {
	return aw.addSuber()
}

func (aw *VIPPortmarAcctWatcher) Unsub(c chan VIPPortmarAcctWatcherMsg) {
	aw.remove(c)
}

func (aw *VIPPortmarAcctWatcher) broadcast(acct *VIPPortmarAccount, err error) {
	if err != nil {
		aw.logger.Error("Fanning Out Account Query Error", "err", err)
	} else {
		aw.logger.Info("Fanning Out Account")
	}
	aw.muxSubbers.Lock()
	defer aw.muxSubbers.Unlock()
	for _, suber := range aw.subbers {
		go func(suber chan VIPPortmarAcctWatcherMsg) {
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				aw.logger.Error("No Reader Of Account Channel Within 1 Second")
			case suber <- VIPPortmarAcctWatcherMsg{acct, err}:
			}
		}(suber)
	}
}

func (aw *VIPPortmarAcctWatcher) watch() {
	for {
		select {
		case <-aw.ctx.Done():
			aw.logger.Info("Watcher Ctx Done", "err", aw.ctx.Err())
			return
		case <-time.After(time.Second * 2):
		}
		acct, err := aw.update()
		aw.broadcast(acct, err)
	}
}

func (aw *VIPPortmarAcctWatcher) Start() error {
	aw.muxClosed.Lock()
	defer aw.muxClosed.Unlock()
	if aw.closed {
		return errors.New("account watcher is closed")
	}
	go aw.watch()
	return nil
}

func (aw *VIPPortmarAcctWatcher) Close() {
	aw.muxClosed.Lock()
	defer aw.muxClosed.Unlock()
	aw.closed = true
	aw.ctxCancel()
}
