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

type AcctWatcherMsg struct {
	Acct *Account
	Err  error
}

type AcctWatcher struct {
	user *bnc.User

	ctx       context.Context
	ctxCancel context.CancelFunc

	muxAcct sync.Mutex
	acct    *Account

	muxSubbers sync.Mutex
	subbers    []chan AcctWatcherMsg

	muxClosed sync.Mutex
	closed    bool

	logger *slog.Logger
}

func NewAcctWatcher(user *bnc.User, logger *slog.Logger) *AcctWatcher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	logger = logger.With("watcher", user.Api().Cex+"_acct")
	ctx, cancel := context.WithCancel(context.Background())
	return &AcctWatcher{
		user:      user,
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
	}
}

func (aw *AcctWatcher) update() (acct *Account, err error) {
	aw.logger.Info("Updating Account")
	_, acct, respErr := QueryAccount(aw.user)
	if respErr.IsNotNil() {
		return nil, respErr.Err
	}
	aw.acct = acct
	return acct, nil
}

func (aw *AcctWatcher) Update() (updating bool, acct *Account, err error) {
	if !aw.muxAcct.TryLock() {
		return true, nil, nil
	}
	defer aw.muxAcct.Unlock()
	acct, err = aw.update()
	return
}

func (aw *AcctWatcher) Acct() (updating bool, acct *Account) {
	if !aw.muxAcct.TryLock() {
		return true, nil
	}
	defer aw.muxAcct.Unlock()
	return false, aw.acct
}

func (aw *AcctWatcher) addSuber() chan AcctWatcherMsg {
	aw.muxSubbers.Lock()
	defer aw.muxSubbers.Unlock()
	c := make(chan AcctWatcherMsg)
	aw.subbers = append(aw.subbers, c)
	return c
}

func (aw *AcctWatcher) remove(c chan AcctWatcherMsg) {
	aw.muxSubbers.Lock()
	defer aw.muxSubbers.Unlock()
	for i, suber := range aw.subbers {
		if suber == c {
			aw.subbers = slices.Delete(aw.subbers, i, i)
			return
		}
	}
}

func (aw *AcctWatcher) Sub() chan AcctWatcherMsg {
	return aw.addSuber()
}

func (aw *AcctWatcher) Unsub(c chan AcctWatcherMsg) {
	aw.remove(c)
}

func (aw *AcctWatcher) broadcast(acct *Account, err error) {
	if err != nil {
		aw.logger.Error("Fanning Out Account Query Error", "err", err)
	} else {
		aw.logger.Info("Fanning Out Account")
	}
	aw.muxSubbers.Lock()
	defer aw.muxSubbers.Unlock()
	for _, suber := range aw.subbers {
		suber := suber
		go func() {
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				aw.logger.Error("No Reader Of Account Channel Within 1 Second")
			case suber <- AcctWatcherMsg{acct, err}:
			}
		}()
	}
}

func (aw *AcctWatcher) watch() {
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

func (aw *AcctWatcher) Start() error {
	aw.muxClosed.Lock()
	defer aw.muxClosed.Unlock()
	if aw.closed {
		return errors.New("account watcher is closed")
	}
	go aw.watch()
	return nil
}

func (aw *AcctWatcher) Close() {
	aw.muxClosed.Lock()
	defer aw.muxClosed.Unlock()
	aw.closed = true
	aw.ctxCancel()
}
