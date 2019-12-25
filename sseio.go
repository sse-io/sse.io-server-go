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

func SetPath(path string) Options {
	return func(s *sseio) {
		s.path = path
	}
}

type SSEIO interface {
	RegisterEventHandler(event string, opts ...HandlerOptions) (EventHandler, error)
	Listen(addr string) error
	ReceiveEvent() chan Event
	Shutdown(context context.Context) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

type sseio struct {
	manager     Manager
	path        string
	enableEvent bool
	eventChan   chan Event

	server *http.Server
}

func NewSSEIO(opts ...Options) SSEIO {
	manager := NewManager()
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

func (s *sseio) httpHandler() http.Handler {
	r := mux.NewRouter()
	r.Handle(s.path, s).Methods("GET")

	return r
}

func (s *sseio) Listen(addr string) error {
	server := &http.Server{Addr: addr, Handler: s.httpHandler()}
	s.server = server
	return server.ListenAndServe()
}

func (s *sseio) Shutdown(context context.Context) error {
	return s.server.Shutdown(context)
}

func (s *sseio) ReceiveEvent() chan Event {
	return s.eventChan
}

func (s *sseio) RegisterEventHandler(event string, opts ...HandlerOptions) (EventHandler, error) {
	if s.enableEvent {
		opts = append(opts, EnableHandlerEvent(s.eventChan))
	}
	handler, err := NewEventHandler(event, s.manager, opts...)
	if err != nil {
		return nil, err
	}
	s.manager.addEventHandler(event, handler)

	return handler, nil
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

	ctx := Context{
		Params: mux.Vars(r),
		Query:  r.URL.Query(),
	}
	for _, event := range events {
		eventHandler := s.manager.clientBindEventHandler(event, client, ctx)
		if eventHandler != nil && eventHandler.Fetch != nil {
			go func(event string) {
				data := eventHandler.Fetch(ctx)
				client.SendMessage(event, data)
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
