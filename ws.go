package main

import (
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

func ws(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("main: error accepting ws conn: %v\n", err)
		return
	}
	defer conn.CloseNow()

	overlayConn, err := NewOverlayConn(conf, conn)
	if err != nil {
		log.Printf("main: error creating overlay conn: %v\n", err)
		return
	}
	go overlayConn.Heartbeat(5 * time.Second)
	<-overlayConn.ctx.Done()
}
