package sseio

import (
	"errors"
	"sync"
)

// EventHandler interface
type EventHandler interface {
	SendMessage(roomId string, message interface{})

	addClientToRoom(roomId string, clientId string)
	removeClientFromRoom(roomId string, clientId string)
}

type room struct {
	lock    sync.RWMutex
	clients map[string]interface{}
}

// HandlerOptions defines option functions to configure the eventHandler.
type HandlerOptions func(e *eventHandler)

// SetFetchFunc will be triggered once a client is connected.
// And the returned data will be send to the client, and nil data will be ignored.
//
// It's optional.
func SetFetchFunc(fetch fetch) HandlerOptions {
	return func(e *eventHandler) {
		e.fetch = fetch
	}
}

// SetGetRoomIdFunc sets the getRoomId function to determine
// which room the incoming client belongs to.
//
// It's required!
func SetGetRoomIdFunc(getRoomId getRoomId) HandlerOptions {
	return func(e *eventHandler) {
		e.getRoomId = getRoomId
	}
}

func enableHandlerEvent(eventChan chan Event) HandlerOptions {
	return func(e *eventHandler) {
		e.eventEnable = true
		e.eventChan = eventChan
	}
}

type eventHandler struct {
	eventEnable bool
	event       string
	eventChan   chan Event
	rooms       sync.Map
	manager     *manager

	getRoomId getRoomId
	fetch     fetch
}

// NewEventHandler will return pointer to eventHandler
func NewEventHandler(event string, manager *manager, opts ...HandlerOptions) (EventHandler, error) {
	e := &eventHandler{
		event:   event,
		manager: manager,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.getRoomId == nil {
		return nil, errors.New("HandlerOptions \"GetRoomId\" is required")
	}

	return e, nil
}

// SendMessage sends data to all the clients in room
func (e *eventHandler) SendMessage(roomId string, data interface{}) {
	v, ok := e.rooms.Load(roomId)
	if !ok {
		return
	}

	if data == nil {
		return
	}

	room := v.(*room)
	room.lock.RLock()
	for clientId := range room.clients {
		client := e.manager.getClient(clientId)
		if client != nil {
			client.sendMessage(e.event, data)
		}
	}
	room.lock.RUnlock()
}

func (e *eventHandler) addClientToRoom(roomId string, clientId string) {
	v, ok := e.rooms.LoadOrStore(roomId, &room{
		clients: map[string]interface{}{
			clientId: nil,
		},
	})
	if !ok {
		if e.eventEnable {
			e.eventChan <- &RoomCreateEvent{
				RoomId: roomId,
			}
		}

		return
	}

	room := v.(*room)
	room.lock.Lock()
	room.clients[clientId] = nil
	room.lock.Unlock()
}

func (e *eventHandler) removeClientFromRoom(roomId string, clientId string) {
	v, ok := e.rooms.Load(roomId)
	if ok {
		room := v.(*room)
		room.lock.Lock()
		delete(room.clients, clientId)

		if len(room.clients) == 0 {
			e.rooms.Delete(roomId)
			if e.eventEnable {
				e.eventChan <- &RoomEmptyEvent{
					RoomId: roomId,
				}
			}
		}

		room.lock.Unlock()
	}
}
