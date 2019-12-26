package sseio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"net/http"
	"runtime/debug"
)

const pingData = `:\n:\n:\n\n`

// Context which contains Params for route variables and Query for url query parameters
type Context struct {
	Params map[string]string
	Query  map[string][]string
}

type getRoomId func(context Context) string
type fetch func(context Context) interface{}

// Options defines option functions to configure the sseio.
type Options func(s *sseio)

// EnableEvent enable the triggered events to be sent the event channel returned by Events method.
func EnableEvent() Options {
	return func(s *sseio) {
		s.enableEvent = true
	}
}

// SetPath set the url path for sseio
func SetPath(path string) Options {
	return func(s *sseio) {
		s.path = path
	}
}

// SSEIO interface.
// It implements http.Handler interface.
type SSEIO interface {
	RegisterEventHandler(event string, opts ...HandlerOptions) (EventHandler, error)
	Listen(addr string) error
	Events() chan Event
	Shutdown(context context.Context) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

type sseio struct {
	manager     *manager
	path        string
	enableEvent bool
	eventChan   chan Event

	server *http.Server
}

// NewSSEIO will return pointer to sseio
func NewSSEIO(opts ...Options) SSEIO {
	manager := newManager()
	s := &sseio{
		path:      "/sseio",
		manager:   manager,
		eventChan: make(chan Event, 1),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Listen calls the ListenAndServe method on http.Server.
// It's used for standalone mode. The server only handles requests for sseio
func (s *sseio) Listen(addr string) error {
	server := &http.Server{Addr: addr, Handler: s.httpHandler()}
	s.server = server
	return server.ListenAndServe()
}

// Shutdown calls the Shutdown method on http.Server.
func (s *sseio) Shutdown(context context.Context) error {
	return s.server.Shutdown(context)
}

// Events returns the Events channel (if enabled)
func (s *sseio) Events() chan Event {
	return s.eventChan
}

// RegisterEventHandler creates an eventHandler for the event
func (s *sseio) RegisterEventHandler(event string, opts ...HandlerOptions) (EventHandler, error) {
	if s.enableEvent {
		opts = append(opts, enableHandlerEvent(s.eventChan))
	}
	handler, err := NewEventHandler(event, s.manager, opts...)
	if err != nil {
		return nil, err
	}
	s.manager.addEventHandler(event, handler)

	return handler, nil
}

// ServeHTTP implements ServeHTTP(ResponseWriter, *Request) to handle requests for sseio
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

	messageChan := make(chan *message, 1)
	client := s.manager.addClient(clientId, messageChan)

	ctx := Context{
		Params: mux.Vars(r),
		Query:  r.URL.Query(),
	}
	for _, event := range events {
		eventHandler := s.manager.clientBindEventHandler(event, client, ctx)
		if eventHandler != nil && eventHandler.fetch != nil {
			go func(event string) {
				data := eventHandler.fetch(ctx)
				client.sendMessage(event, data)
			}(event)
		}
	}

	if s.enableEvent {
		s.eventChan <- &ConnectionCreateEvent{
			ClientId: clientId,
			Context:  ctx,
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
		// connection closed by client
		case <-r.Context().Done():
			logrus.Debug("client close")
			return
		case data := <-messageChan:
			// chan has been closed
			if data == nil {
				logrus.Debug("server close")
				return
			}

			if data.event == PingEvent {
				fmt.Fprint(w, pingData)
			} else {
				str, ok := data.data.(string)
				if ok {
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.event, str)
				} else {
					var byteData []byte
					byteData, _ = json.Marshal(data.data)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.event, byteData)
				}
			}
			flusher.Flush()
		}
	}
}

func (s *sseio) httpHandler() http.Handler {
	r := mux.NewRouter()
	r.Handle(s.path, s).Methods("GET")

	return r
}

func errorHandler(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	res := make(map[string]interface{})
	res["message"] = msg
	json.NewEncoder(w).Encode(res)
}
