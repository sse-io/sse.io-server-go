package sseio

import (
	"sync"
	"time"
)

const pingEvent = "ping"

type message struct {
	event string
	data  interface{}
}

type roomInfo struct {
	id    string
	event string
}

type client struct {
	id             string
	dying          bool
	messageChan    chan *message
	channelLock    sync.RWMutex
	rooms          []*roomInfo
	heartbeatTimer *time.Timer
}

func newClient(id string, messageChan chan *message) *client {
	c := &client{
		id:          id,
		messageChan: messageChan,
	}
	c.startHeartbeat()

	return c
}

func (c *client) getId() string {
	return c.id
}

func (c *client) close() {
	c.channelLock.Lock()
	defer c.channelLock.Unlock()
	if !c.dying {
		c.dying = true
		close(c.messageChan)
	}
}

func (c *client) isDying() bool {
	return c.dying == true
}

func (c *client) sendMessage(event string, data interface{}) {
	if data != nil {
		c.channelLock.RLock()
		defer c.channelLock.RUnlock()
		if !c.dying {
			c.messageChan <- &message{event: event, data: data}
		}
	}
}

func (c *client) addRoom(event string, roomId string) {
	c.rooms = append(c.rooms, &roomInfo{
		id:    roomId,
		event: event,
	})
}

func (c *client) getRooms() []*roomInfo {
	return c.rooms
}

func (c *client) startHeartbeat() {
	go func() {
		timer := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-timer.C:
				c.sendMessage(pingEvent, nil)
			}
		}
	}()
}
