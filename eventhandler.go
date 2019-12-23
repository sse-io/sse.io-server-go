package sseio

import (
	"sync"
)

type EventHandler interface {
	SendMessage(roomId string, message interface{})
	AddClientToRoom(roomId string, clientId string)
	RemoveClientFromRoom(roomId string, clientId string)
	EnableEvents()
	Events() chan Event
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

type eventHandler struct {
	eventEnable bool
	event       string
	eventChan   chan Event
	rooms       sync.Map
	manager     Manager

	GetRoomId GetRoomId
	Fetch     Fetch
}

func NewEventHandler(event string, manager Manager, getRoomId GetRoomId) EventHandler {
	return &eventHandler{
		event:     event,
		manager:   manager,
		GetRoomId: getRoomId,
	}
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

func (e *eventHandler) AddClientToRoom(roomId string, clientId string) {
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

func (e *eventHandler) RemoveClientFromRoom(roomId string, clientId string) {
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

func (e *eventHandler) EnableEvents() {
	e.eventEnable = true
}

func (e *eventHandler) Events() chan Event {
	return e.eventChan
}
