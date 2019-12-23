package sseio

import (
	"sync"
)

type Manager interface {
	addEventHandler(event string, eventHandler EventHandler)
	addClient(clientId string, messageChan chan *Message) Client
	getClient(clientId string) Client
	removeClient(clientId string)
	clientBindEventHandler(event string, client Client, context Context) *eventHandler
}

type manager struct {
	eventHandlers sync.Map
	clients       sync.Map
}

func NewManager() Manager {
	return &manager{}
}

func (m *manager) addEventHandler(event string, eventHandler EventHandler) {
	m.eventHandlers.Store(event, eventHandler)
}

func (m *manager) addClient(clientId string, messageChan chan *Message) Client {
	client := NewClient(clientId, messageChan)
	m.clients.Store(clientId, client)
	return client
}

func (m *manager) getClient(clientId string) Client {
	client, ok := m.clients.Load(clientId)
	if !ok {
		return nil
	}

	return client.(Client)
}

func (m *manager) removeClient(clientId string) {
	v, ok := m.clients.Load(clientId)
	if !ok {
		return
	}

	client := v.(Client)
	rooms := client.GetRooms()
	for _, room := range rooms {
		v, ok := m.eventHandlers.Load(room.event)
		if !ok {
			continue
		}

		v.(EventHandler).RemoveClientFromRoom(room.id, clientId)
	}
	client.Close()
	m.clients.Delete(clientId)
}

func (m *manager) clientBindEventHandler(event string, client Client, context Context) *eventHandler {
	v, ok := m.eventHandlers.Load(event)
	if !ok {
		return nil
	}

	eventHandler := v.(*eventHandler)
	roomId := eventHandler.GetRoomId(context)
	eventHandler.AddClientToRoom(roomId, client.GetId())
	client.AddRoom(event, roomId)
	return eventHandler
}
