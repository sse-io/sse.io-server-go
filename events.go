package sseio

type Event interface {
	String() string
}

// RoomCreateEvent will be triggered when a room is created by client.
type RoomCreateEvent struct {
	RoomId string
}

func (e *RoomCreateEvent) String() string {
	return "room:create"
}

// RoomEmptyEvent will be triggered when there is no client in the room. And the room will be deleted.
type RoomEmptyEvent struct {
	RoomId string
}

func (e *RoomEmptyEvent) String() string {
	return "room:empty"
}

// ConnectionCreateEvent will be triggered when a connection is created.
type ConnectionCreateEvent struct {
	ClientId string
	Context  Context
}

func (c *ConnectionCreateEvent) String() string {
	return "connection:create"
}

// ConnectionCreateEvent will be triggered when a connection is closed.
type ConnectionCloseEvent struct {
	ClientId string
}

func (c *ConnectionCloseEvent) String() string {
	return "connection:close"
}
