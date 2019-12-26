package sseio

import (
	"sync"
)

type manager struct {
	eventHandlers sync.Map
	clients       sync.Map
}

func newManager() *manager {
	return &manager{}
}

func (m *manager) addEventHandler(event string, eventHandler EventHandler) {
	m.eventHandlers.Store(event, eventHandler)
}

func (m *manager) addClient(clientId string, messageChan chan *message) *client {
	client := newClient(clientId, messageChan)
	m.clients.Store(clientId, client)
	return client
}

func (m *manager) getClient(clientId string) *client {
	v, ok := m.clients.Load(clientId)
	if !ok {
		return nil
	}

	return v.(*client)
}

func (m *manager) removeClient(clientId string) {
	v, ok := m.clients.Load(clientId)
	if !ok {
		return
	}

	client := v.(*client)
	rooms := client.getRooms()
	for _, room := range rooms {
		v, ok := m.eventHandlers.Load(room.event)
		if !ok {
			continue
		}

		v.(EventHandler).removeClientFromRoom(room.id, clientId)
	}
	client.close()
	m.clients.Delete(clientId)
}

func (m *manager) clientBindEventHandler(event string, client *client, context Context) *eventHandler {
	v, ok := m.eventHandlers.Load(event)
	if !ok {
		return nil
	}

	eventHandler := v.(*eventHandler)
	roomId := eventHandler.getRoomId(context)
	eventHandler.addClientToRoom(roomId, client.getId())
	client.addRoom(event, roomId)
	return eventHandler
}
