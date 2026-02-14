package stream

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

const (
	redisChannel      = "messenger:stream:notify"
	redisDeliveryCh   = "messenger:stream:delivery"
	redisReadCh       = "messenger:stream:read"
)

// Hub broadcasts new message events to connected WebSocket clients by recipient.
// With Redis: publishes to Redis Pub/Sub so multiple server instances can notify their local clients.
// Without Redis: in-memory only (single-node).
type Hub struct {
	mu             sync.RWMutex
	clients        map[string]map[*Client]struct{} // recipient -> clients
	broadcast      chan *Event
	typingBroadcast chan *TypingEvent

	redis     *redis.Client
	pubsub    *redis.PubSub
	ctx       context.Context
	cancelCtx context.CancelFunc
}

type Event struct {
	Recipient string
	EventID   string
}

type Client struct {
	Recipient string
	Sender    string // user address (for typing)
	Send      chan []byte
}

// NewHub creates an in-memory Hub (single-node).
func NewHub() *Hub {
	return NewHubWithRedis("")
}

// NewHubWithRedis creates a Hub. If redisURL is non-empty, uses Redis Pub/Sub for multi-node.
func NewHubWithRedis(redisURL string) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{
		clients:         make(map[string]map[*Client]struct{}),
		broadcast:       make(chan *Event, 256),
		typingBroadcast: make(chan *TypingEvent, 64),
		ctx:             ctx,
		cancelCtx:       cancel,
	}

	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Printf("[stream] invalid Redis URL: %v, using in-memory only", err)
		} else {
			h.redis = redis.NewClient(opts)
			if err := h.redis.Ping(ctx).Err(); err != nil {
				log.Printf("[stream] Redis ping failed: %v, using in-memory only", err)
				h.redis = nil
			} else {
				h.pubsub = h.redis.Subscribe(ctx, redisChannel, redisTypingChannel, redisDeliveryCh, redisReadCh)
				go h.runRedisSubscriber()
				log.Printf("[stream] Redis Pub/Sub enabled for multi-node")
			}
		}
	}

	return h
}

func (h *Hub) runRedisSubscriber() {
	ch := h.pubsub.Channel()
	for {
		select {
		case <-h.ctx.Done():
			if h.pubsub != nil {
				_ = h.pubsub.Close()
			}
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch msg.Channel {
			case redisTypingChannel:
				var te TypingEvent
				if err := json.Unmarshal([]byte(msg.Payload), &te); err == nil {
					h.pushTypingToLocal(te)
				}
			case redisDeliveryCh:
				var ev struct {
					Recipient string `json:"recipient"`
					EventID   string `json:"event_id"`
					Status    string `json:"status"`
				}
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
					h.pushDeliveryToLocal(ev.Recipient, ev.EventID, ev.Status)
				}
			case redisReadCh:
				var ev struct {
					Recipient string `json:"recipient"`
					EventID   string `json:"event_id"`
					ReadAt    string `json:"read_at"`
				}
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
					h.pushReadToLocal(ev.Recipient, ev.EventID, ev.ReadAt)
				}
			default:
				var ev Event
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
					h.pushToLocal(ev)
				}
			}
		}
	}
}

func (h *Hub) pushToLocal(ev Event) {
	select {
	case h.broadcast <- &ev:
	default:
	}
}

func (h *Hub) pushReadToLocal(recipient, eventID, readAt string) {
	h.mu.RLock()
	clients := h.clients[recipient]
	h.mu.RUnlock()
	if clients == nil {
		return
	}
	msg, _ := json.Marshal(map[string]string{"type": "read", "event_id": eventID, "read_at": readAt})
	for c := range clients {
		select {
		case c.Send <- msg:
		default:
		}
	}
}

func (h *Hub) pushDeliveryToLocal(recipient, eventID, status string) {
	h.mu.RLock()
	clients := h.clients[recipient]
	h.mu.RUnlock()
	if clients == nil {
		return
	}
	msg, _ := json.Marshal(map[string]string{"type": "delivery", "event_id": eventID, "status": status})
	for c := range clients {
		select {
		case c.Send <- msg:
		default:
		}
	}
}

func (h *Hub) pushTypingToLocal(te TypingEvent) {
	select {
	case h.typingBroadcast <- &te:
	default:
	}
}

func (h *Hub) Run() {
	for {
		select {
		case ev := <-h.broadcast:
			h.mu.RLock()
			clients := h.clients[ev.Recipient]
			h.mu.RUnlock()
			if clients != nil {
				msg, _ := json.Marshal(map[string]string{"event_id": ev.EventID, "type": "new_message"})
				for c := range clients {
					select {
					case c.Send <- msg:
					default:
					}
				}
			}
		case te := <-h.typingBroadcast:
			h.mu.RLock()
			clients := h.clients[te.Recipient]
			h.mu.RUnlock()
			if clients != nil {
				payload := map[string]interface{}{"type": "typing", "sender": te.Sender, "typing": te.Typing}
				if te.Room != "" {
					payload["room"] = te.Room
				}
				msg, _ := json.Marshal(payload)
				for c := range clients {
					select {
					case c.Send <- msg:
					default:
					}
				}
			}
		case <-h.ctx.Done():
			return
		}
	}
}

// NotifyRead notifies the sender that their message was read by the recipient.
func (h *Hub) NotifyRead(senderAddr, eventID, readAt string) {
	ev := map[string]string{"recipient": senderAddr, "event_id": eventID, "read_at": readAt}
	if h.redis != nil {
		payload, _ := json.Marshal(ev)
		_ = h.redis.Publish(h.ctx, redisReadCh, payload).Err()
	}
	h.pushReadToLocal(senderAddr, eventID, readAt)
}

// NotifyDelivery notifies the sender that their message was delivered to the recipient.
func (h *Hub) NotifyDelivery(senderAddr, eventID, status string) {
	ev := map[string]string{"recipient": senderAddr, "event_id": eventID, "status": status}
	if h.redis != nil {
		payload, _ := json.Marshal(ev)
		_ = h.redis.Publish(h.ctx, redisDeliveryCh, payload).Err()
	}
	h.pushDeliveryToLocal(senderAddr, eventID, status)
}

// Notify sends a new message event. With Redis: publishes to all nodes; always pushes to local broadcast.
func (h *Hub) Notify(recipient, eventID string) {
	ev := &Event{Recipient: recipient, EventID: eventID}

	if h.redis != nil {
		payload, _ := json.Marshal(ev)
		if err := h.redis.Publish(h.ctx, redisChannel, payload).Err(); err != nil {
			log.Printf("[stream] Redis publish: %v", err)
		}
	}

	h.pushToLocal(*ev)
}

// TypingEvent is sent to recipient when someone is typing.
// For rooms: Room is set, Recipient is each member's address.
type TypingEvent struct {
	Recipient string
	Sender    string
	Typing    bool
	Room      string // optional: room address when typing in a room
}

const redisTypingChannel = "messenger:stream:typing"

// NotifyTyping broadcasts typing status to recipient's clients.
// Use room="" for DM; for rooms call NotifyTypingToMember for each member.
func (h *Hub) NotifyTyping(recipient, sender string, typing bool) {
	h.notifyTyping(recipient, sender, typing, "")
}

// NotifyTypingToMember broadcasts typing in a room to one member. roomAddr is the room address.
func (h *Hub) NotifyTypingToMember(memberAddr, sender string, typing bool, roomAddr string) {
	h.notifyTyping(memberAddr, sender, typing, roomAddr)
}

func (h *Hub) notifyTyping(recipient, sender string, typing bool, room string) {
	ev := &TypingEvent{Recipient: recipient, Sender: sender, Typing: typing, Room: room}
	if h.redis != nil {
		payload, _ := json.Marshal(ev)
		_ = h.redis.Publish(h.ctx, redisTypingChannel, payload).Err()
	}
	h.pushTypingToLocal(*ev)
}

func (h *Hub) Register(recipient, sender string, send chan []byte) *Client {
	c := &Client{Recipient: recipient, Sender: sender, Send: send}
	h.mu.Lock()
	if h.clients[c.Recipient] == nil {
		h.clients[c.Recipient] = make(map[*Client]struct{})
	}
	h.clients[c.Recipient][c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.clients[c.Recipient]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.clients, c.Recipient)
		}
	}
}

// Close stops Redis subscriber. Call before shutdown.
func (h *Hub) Close() {
	h.cancelCtx()
	if h.redis != nil {
		_ = h.redis.Close()
	}
}
