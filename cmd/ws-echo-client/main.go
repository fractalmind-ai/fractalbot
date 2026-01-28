package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
	"github.com/gorilla/websocket"
)

func main() {
	url := flag.String("url", "ws://127.0.0.1:18789/ws", "websocket server URL")
	flag.Parse()

	conn, _, err := websocket.DefaultDialer.Dial(*url, nil)
	if err != nil {
		log.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	req := protocol.Message{
		Kind:   protocol.MessageKindEvent,
		Action: protocol.ActionEcho,
		Data: map[string]string{
			"text": "hello",
		},
	}

	if err := conn.WriteJSON(&req); err != nil {
		log.Fatalf("Write failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var resp protocol.Message
	if err := conn.ReadJSON(&resp); err != nil {
		log.Fatalf("Read failed: %v", err)
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("Marshal failed: %v", err)
	}

	fmt.Println(string(payload))
}
