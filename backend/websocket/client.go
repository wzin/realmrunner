package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wzin/realmrunner/server"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	serverID string
	manager  *server.Manager
	done     chan struct{}
}

type WSMessage struct {
	Type      string `json:"type"`
	Message   string `json:"message,omitempty"`
	Status    string `json:"status,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

func HandleConnection(w http.ResponseWriter, r *http.Request, hub *Hub, manager *server.Manager, serverID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		serverID: serverID,
		manager:  manager,
		done:     make(chan struct{}),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
	go client.tailLogs()
}

func (c *Client) readPump() {
	defer func() {
		close(c.done) // Signal all goroutines to stop
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) tailLogs() {
	// Get server info
	srv, err := c.manager.GetServer(c.serverID)
	if err != nil {
		return
	}

	// Send initial status
	c.sendMessage(WSMessage{
		Type:   "status",
		Status: srv.Status,
	})

	serverDir := c.manager.GetServerDir(c.serverID)

	// If server is running, tail logs in real-time
	if srv.Status == server.StatusRunning {
		process, exists := c.manager.GetProcess(c.serverID)
		if !exists {
			return
		}

		logChan, err := process.TailLogs(serverDir)
		if err != nil {
			log.Printf("Failed to tail logs: %v", err)
			return
		}

		for {
			select {
			case <-c.done:
				// Client disconnected, stop tailing
				return
			case line, ok := <-logChan:
				if !ok {
					// Log channel closed
					return
				}
				c.sendMessage(WSMessage{
					Type:      "log",
					Message:   line,
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}
	} else {
		// Server is stopped, send historical logs
		logs, err := server.ReadHistoricalLogs(serverDir)
		if err != nil {
			log.Printf("Failed to read historical logs: %v", err)
			return
		}

		// Send all historical logs
		for _, line := range logs {
			select {
			case <-c.done:
				// Client disconnected, stop sending
				return
			default:
				c.sendMessage(WSMessage{
					Type:      "log",
					Message:   line,
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}
	}
}

func (c *Client) sendMessage(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	// Safely send without panicking if channel is closed
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in sendMessage: %v", r)
		}
	}()

	select {
	case c.send <- data:
	default:
		// Channel full, skip message
	}
}
