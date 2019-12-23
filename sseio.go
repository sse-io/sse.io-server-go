package sseio

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"net/http"
	"runtime/debug"
)

const PingData = `:\n:\n:\n\n`

type Context struct {
	Params map[string]string
	Query  map[string][]string
}

type GetRoomId func(context Context) string
type Fetch func(context Context) interface{}

type Options func(s *sseio)

func EnableEvent() Options {
	return func(s *sseio) {
		s.enableEvent = true
	}
}

type SSEIO interface {
	RegisterEventHandler(event string, getRoomId GetRoomId, opts ...HandlerOptions) EventHandler
	HttpHandler() http.Handler
	Listen(addr string) error
	ReceiveEvent() chan Event
}

type sseio struct {
	manager     Manager
	path        string
	enableEvent bool
	eventChan   chan Event
}

func NewSSEIO(path string, opts ...Options) SSEIO {
	manager := NewManager()
	s := &sseio{
		path:      path,
		manager:   manager,
		eventChan: make(chan Event, 1),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *sseio) HttpHandler() http.Handler {
	r := mux.NewRouter()
	r.Handle(s.path, s).Methods("GET")

	return r
}

func (s *sseio) Listen(addr string) error {
	return http.ListenAndServe(addr, s.HttpHandler())
}

func (s *sseio) ReceiveEvent() chan Event {
	return s.eventChan
}

func (s *sseio) RegisterEventHandler(event string, getRoomId GetRoomId, opts ...HandlerOptions) EventHandler {
	handler := NewEventHandler(event, s.manager, getRoomId)
	s.manager.addEventHandler(event, handler)

	for _, opt := range opts {
		opt(handler.(*eventHandler))
	}

	return handler
}

func (s *sseio) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientIds, ok := r.URL.Query()["clientId"]
	if !ok || len(clientIds) < 1 {
		errorHandler(w, http.StatusBadRequest, "clientId is required")
		return
	}
	clientId := clientIds[0]

	events, ok := r.URL.Query()["events"]
	if !ok || len(events) < 1 {
		errorHandler(w, http.StatusBadRequest, "events is required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		errorHandler(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	messageChan := make(chan *Message, 1)
	client := s.manager.addClient(clientId, messageChan)

	context := Context{
		Params: mux.Vars(r),
		Query:  r.URL.Query(),
	}
	for _, event := range events {
		eventHandler := s.manager.clientBindEventHandler(event, client, context)
		if eventHandler != nil && eventHandler.Fetch != nil {
			go func(event string) {
				data := eventHandler.Fetch(context)
				client.SendMessage(event, data)
			}(event)
		}
	}

	if s.enableEvent {
		s.eventChan <- &ConnectionCreateEvent{
			ClientId: clientId,
			Context:  context,
		}
	}

	defer func() {
		s.manager.removeClient(clientId)
		if s.enableEvent {
			s.eventChan <- &ConnectionCloseEvent{
				ClientId: clientId,
			}
		}
		if err := recover(); err != nil {
			logrus.WithFields(logrus.Fields{
				"tag":   "panic_error",
				"err":   err,
				"stack": string(debug.Stack()),
			}).Error(err)
			errorHandler(w, http.StatusInternalServerError, "internal error")
		}
	}()

	for {
		select {
		// client 端关闭
		case <-r.Context().Done():
			logrus.Debug("client close")
			return
		case data := <-messageChan:
			// server端关闭, chan 已经被 closed
			if data == nil {
				logrus.Debug("server close")
				return
			}

			if data.Event == PingEvent {
				fmt.Fprint(w, PingData)
			} else {
				str, ok := data.Data.(string)
				if ok {
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.Event, str)
				} else {
					var byteData []byte
					byteData, _ = json.Marshal(data.Data)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.Event, byteData)
				}
			}
			flusher.Flush()
		}
	}
}

func errorHandler(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	res := make(map[string]interface{})
	res["message"] = msg
	json.NewEncoder(w).Encode(res)
}
