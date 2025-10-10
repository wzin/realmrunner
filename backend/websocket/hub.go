package websocket

import (
	"sync"
)

type Hub struct {
	clients    map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Message
	mu         sync.RWMutex
}

type Message struct {
	ServerID string
	Data     []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.serverID] == nil {
				h.clients[client.serverID] = make(map[*Client]bool)
			}
			h.clients[client.serverID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.serverID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.clients, client.serverID)
					}
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := h.clients[message.ServerID]
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.send <- message.Data:
				default:
					h.mu.Lock()
					close(client.send)
					delete(h.clients[message.ServerID], client)
					h.mu.Unlock()
				}
			}
		}
	}
}

func (h *Hub) Broadcast(serverID string, data []byte) {
	h.broadcast <- &Message{
		ServerID: serverID,
		Data:     data,
	}
}
