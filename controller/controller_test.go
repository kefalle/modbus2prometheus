package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/simonvetter/modbus"
)

func TestRunStopsAfterContextCancellation(t *testing.T) {
	read := make(chan struct{})
	var readOnce sync.Once
	client := &fakeRegisterClient{
		readRegisterFn: func(uint16, modbus.RegType) (uint16, error) {
			readOnce.Do(func() { close(read) })
			return 42, nil
		},
	}
	ctrl := newWithClient(Configuration{
		PollingTime: time.Hour,
		MaxAttempts: 1,
	}, client, metrics.NewSet())
	ctrl.AddTag(&Tag{Name: "temperature", Method: READ_UINT})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ctrl.Run(ctx)
	}()

	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("Run did not read a register")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}

	if client.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", client.closeCalls)
	}
}

func TestRunStopsAfterReconnectLimit(t *testing.T) {
	readErr := errors.New("read failed")
	openErr := errors.New("open failed")
	client := &fakeRegisterClient{
		readRegisterFn: func(uint16, modbus.RegType) (uint16, error) {
			return 0, readErr
		},
		openFn: func() error {
			return openErr
		},
	}
	ctrl := newWithClient(Configuration{MaxAttempts: 3}, client, metrics.NewSet())
	ctrl.AddTag(&Tag{Name: "temperature", Method: READ_UINT})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ctrl.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, openErr) {
			t.Fatalf("Run error = %v, want error wrapping %v", err, openErr)
		}
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("Run did not stop after reconnect limit")
	}
	cancel()

	if client.openCalls != 3 {
		t.Fatalf("Open calls = %d, want 3", client.openCalls)
	}
}
