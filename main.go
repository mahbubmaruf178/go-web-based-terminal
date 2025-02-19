package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/aymanbagabas/go-pty"
	"github.com/gorilla/websocket"
)

var (
	terminals = make(map[string]*pty.Pty)
	mu        sync.Mutex
	upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	termID := r.URL.Query().Get("id")
	if termID == "" {
		log.Println("Missing terminal ID")
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Println("Failed to get user home directory:", err)
		return
	}

	pt, err := pty.New()
	if err != nil {
		log.Println("Failed to create PTY:", err)
		return
	}
	defer pt.Close()

	// Set initial terminal size to a large value
	if err := pt.Resize(200, 50); err != nil {
		log.Println("Failed to set initial terminal size:", err)
	}

	cmd := pt.Command("cmd.exe")
	cmd.Dir = homeDir

	if err := cmd.Start(); err != nil {
		log.Println("Failed to start shell:", err)
		return
	}

	mu.Lock()
	terminals[termID] = &pt
	mu.Unlock()

	// Initialize terminal and change directory
	go func() {
		pt.Write([]byte{0x0D, 0x0A})
		pt.Write([]byte("cd /d " + homeDir + "\r\n"))
		pt.Write([]byte("cls\r\n"))
	}()

	outputChan := make(chan []byte, 100)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := pt.Read(buf)
			if err != nil {
				log.Println("PTY read error:", err)
				close(outputChan)
				return
			}
			outputChan <- buf[:n]
		}
	}()

	go func() {
		initComplete := false
		for data := range outputChan {
			if !initComplete {
				if len(data) > 0 && data[len(data)-1] == '>' {
					initComplete = true
				}
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Println("WebSocket write error:", err)
				return
			}
		}
	}()

	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			break
		}

		// Handle resize messages
		if messageType == websocket.TextMessage && len(msg) > 6 && string(msg[:6]) == "RESIZE" {
			// Parse resize message and update terminal size
			var cols, rows int
			_, err := fmt.Sscanf(string(msg), "RESIZE:%d:%d", &cols, &rows)
			if err == nil {
				pt.Resize(uint16(cols), uint16(rows))
			}
			continue
		}

		if string(msg) == "CLOSE" {
			log.Printf("Closing terminal: %s", termID)
			mu.Lock()
			delete(terminals, termID)
			mu.Unlock()
			pt.Close()
			cmd.Process.Signal(os.Kill)
			break
		}

		pt.Write(msg)
	}

	mu.Lock()
	delete(terminals, termID)
	mu.Unlock()
}

func main() {
	http.HandleFunc("/ws", handleWS)
	http.Handle("/", http.FileServer(http.Dir("./public")))
	log.Println("Server started on localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
