package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type OverlaySession struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	timer   atomic.Pointer[time.Timer]
	timeout time.Duration
}

func NewOverlaySession(parentCtx context.Context, conn *websocket.Conn, timeout time.Duration) *OverlaySession {
	s := &OverlaySession{}
	s.conn = conn
	s.timeout = timeout

	ctx, cancel := context.WithCancel(parentCtx)
	s.ctx = ctx
	s.cancel = cancel

	t := time.AfterFunc(timeout, func() {
		conn.CloseNow()
	})
	s.timer.Store(t)

	return s
}

func (s *OverlaySession) bumpActivity() {
	if t := s.timer.Load(); t != nil {
		t.Reset(s.timeout)
	}
}

func (s *OverlaySession) ReadJSON[T any](v *T) error {
	err := wsjson.Read(s.ctx, s.conn, v)
	if err == nil {
		s.bumpActivity()
	}
	return err
}

func (s *OverlaySession) WriteJSON[T any](v T) error {
	err := wsjson.Write(s.ctx, s.conn, v)
	if err == nil {
		s.bumpActivity()
	}
	return err
}

func (s *OverlaySession) Close() {
	if t := s.timer.Swap(nil); t != nil {
		t.Stop()
	}
	s.conn.Close(websocket.StatusNormalClosure, "goodbye")
}
