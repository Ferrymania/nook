package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/service/cloud"
)

type accountProbeProvider struct {
	cloud.Provider
	ping func(context.Context) error
}

func (p accountProbeProvider) Ping(ctx context.Context) error { return p.ping(ctx) }

func TestProbeStrmAccountBoundsRequestWithoutCancelingCaller(t *testing.T) {
	parent := context.Background()
	started := time.Now()
	var requestCtx context.Context
	provider := accountProbeProvider{ping: func(ctx context.Context) error {
		requestCtx = ctx
		deadline, ok := ctx.Deadline()
		if !ok || deadline.After(started.Add(21*time.Second)) {
			t.Fatalf("probe must finish before the frontend's 30-second timeout, deadline = %v", deadline)
		}
		return nil
	}}
	if err := probeStrmAccount(parent, provider); err != nil {
		t.Fatal(err)
	}
	if requestCtx.Err() != context.Canceled {
		t.Fatal("probe context was not released after completion")
	}
	if parent.Err() != nil {
		t.Fatal("caller context must remain usable to persist the test result")
	}
}

func TestProbeStrmAccountHonorsEarlierDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	provider := accountProbeProvider{ping: func(probeCtx context.Context) error {
		deadline, _ := probeCtx.Deadline()
		parentDeadline, _ := ctx.Deadline()
		if !deadline.Equal(parentDeadline) {
			t.Fatal("probe extended the caller's deadline")
		}
		<-probeCtx.Done()
		return probeCtx.Err()
	}}
	err := probeStrmAccount(ctx, provider)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "连接测试超时") {
		t.Fatalf("want an actionable timeout, got %v", err)
	}
}

func TestProbeStrmAccountKeepsOtherErrors(t *testing.T) {
	for _, want := range []error{context.Canceled, errors.New("openlist: invalid token")} {
		t.Run(want.Error(), func(t *testing.T) {
			provider := accountProbeProvider{ping: func(context.Context) error { return want }}
			if got := probeStrmAccount(context.Background(), provider); got != want {
				t.Fatalf("got %v, want original error %v", got, want)
			}
		})
	}
}
