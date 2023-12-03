package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
)

var (
	upgrader                                 = newUpgrader()
	remoteAddrs map[*websocket.Conn]struct{} = map[*websocket.Conn]struct{}{}
	allowList                                = flag.String("allowlist", "", "allowed origin to access server (comma separated)")
	dev                                      = flag.Bool("dev", false, "determine if app running in dev mode")
)

func newUpgrader() *websocket.Upgrader {
	u := websocket.NewUpgrader()
	u.CheckOrigin = func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		isAllowed := false
		for _, allow := range strings.Split(*allowList, ",") {
			if !*dev && strings.Contains(origin, "localhost") {
				continue
			}
			isAllowed = strings.Contains(origin, allow)
		}
		return isAllowed
	}
	u.OnOpen(func(c *websocket.Conn) {
		remoteAddrs[c] = struct{}{}
	})
	u.OnMessage(func(c *websocket.Conn, messageType websocket.MessageType, data []byte) {
		for remoteAddr := range remoteAddrs {
			remoteAddr.WriteMessage(messageType, data)
		}
	})
	u.OnClose(func(c *websocket.Conn, err error) {
		delete(remoteAddrs, c)
	})

	return u
}

func onWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("Upgraded:", conn.RemoteAddr().String())
}

func main() {
	flag.Parse()
	mux := &http.ServeMux{}
	mux.HandleFunc("/ws", onWebsocket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		type Response struct {
			IsSuccess bool `json:"is_success"`
		}
		json.NewEncoder(w).Encode(Response{IsSuccess: true})
	})
	engine := nbhttp.NewEngine(nbhttp.Config{
		Network:                 "tcp",
		Addrs:                   []string{":8080"},
		MaxLoad:                 1000000,
		ReleaseWebsocketPayload: true,
		Handler:                 mux,
	})

	err := engine.Start()
	if err != nil {
		fmt.Printf("nbio.Start failed: %v\n", err)
		return
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	engine.Shutdown(ctx)
}
