package main

import (
	"log"
	"net/http"

	"github.com/aymanbagabas/go-pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // allow any origin
}

func terminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	pt, err := pty.New()
	if err != nil {
		log.Println("PTY create error:", err)
		return
	}
	defer pt.Close()

	cmd := pt.Command("cmd.exe")
	if err := cmd.Start(); err != nil {
		log.Println("cmd start error:", err)
		return
	}

	// Send PTY output to browser
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := pt.Read(buf)
			if err != nil {
				break
			}
			conn.WriteMessage(websocket.TextMessage, buf[:n])
		}
	}()

	// Receive browser input and send to PTY
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		pt.Write(msg)
	}
}
func main() {
	http.HandleFunc("/ws", terminalWS)
	http.Handle("/", http.FileServer(http.Dir("./public"))) // serve index.html
	log.Println("Server started on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
