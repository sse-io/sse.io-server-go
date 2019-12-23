package sseio

import (
	"sync"
	"time"
)

const PingEvent = "ping"

type Message struct {
	Event string
	Data  interface{}
}

type Client interface {
	IsDying() bool
	Close()
	SendMessage(event string, data interface{})
	AddRoom(event string, roomId string)
	GetRooms() []*roomInfo
	GetId() string
}

type roomInfo struct {
	id    string
	event string
}

type client struct {
	id             string
	dying          bool
	messageChan    chan *Message
	channelLock    sync.RWMutex
	rooms          []*roomInfo
	heartbeatTimer *time.Timer
}

func NewClient(id string, messageChan chan *Message) Client {
	c := &client{
		id:          id,
		messageChan: messageChan,
	}
	c.startHeartbeat()

	return c
}

func (c *client) GetId() string {
	return c.id
}

func (c *client) Close() {
	c.channelLock.Lock()
	defer c.channelLock.Unlock()
	if !c.dying {
		c.dying = true
		close(c.messageChan)
	}
}

func (c *client) IsDying() bool {
	return c.dying == true
}

func (c *client) SendMessage(event string, data interface{}) {
	if data != nil {
		c.channelLock.RLock()
		defer c.channelLock.RUnlock()
		if !c.dying {
			c.messageChan <- &Message{Event: event, Data: data}
		}
	}
}

func (c *client) AddRoom(event string, roomId string) {
	c.rooms = append(c.rooms, &roomInfo{
		id:    roomId,
		event: event,
	})
}

func (c *client) GetRooms() []*roomInfo {
	return c.rooms
}

func (c *client) startHeartbeat() {
	go func() {
		timer := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-timer.C:
				c.SendMessage(PingEvent, nil)
			}
		}
	}()
}
