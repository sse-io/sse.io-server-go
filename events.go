package sseio

type Event interface {
	String() string
}

type RoomCreateEvent struct {
	RoomId string
}

func (e *RoomCreateEvent) String() string {
	return "room:create"
}

type RoomEmptyEvent struct {
	RoomId string
}

func (e *RoomEmptyEvent) String() string {
	return "room:empty"
}

type ConnectionCreateEvent struct {
	ClientId string
	Context  Context
}

func (c *ConnectionCreateEvent) String() string {
	return "connection:create"
}

type ConnectionCloseEvent struct {
	ClientId string
}

func (c *ConnectionCloseEvent) String() string {
	return "connection:close"
}
