package sseio

import (
	"errors"
	"sync"
)

type EventHandler interface {
	SendMessage(roomId string, message interface{})

	addClientToRoom(roomId string, clientId string)
	removeClientFromRoom(roomId string, clientId string)
}

type room struct {
	lock    sync.RWMutex
	clients map[string]interface{}
}

type HandlerOptions func(e *eventHandler)

func SetFetchFunc(fetch Fetch) HandlerOptions {
	return func(e *eventHandler) {
		e.Fetch = fetch
	}
}

func SetGetRoomIdFunc(getRoomId GetRoomId) HandlerOptions {
	return func(e *eventHandler) {
		e.GetRoomId = getRoomId
	}
}

func EnableHandlerEvent(eventChan chan Event) HandlerOptions {
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
	manager     Manager

	GetRoomId GetRoomId
	Fetch     Fetch
}

func NewEventHandler(event string, manager Manager, opts ...HandlerOptions) (EventHandler, error) {
	e := &eventHandler{
		event:   event,
		manager: manager,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.GetRoomId == nil {
		return nil, errors.New("HandlerOptions \"GetRoomId\" is required")
	}

	return e, nil
}

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
			client.SendMessage(e.event, data)
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
