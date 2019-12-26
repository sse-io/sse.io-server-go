package sseio

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/r3labs/sse"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	PORT     = 9988
	TestPath = "/files/{guid}/pull"

	Event1 = "event1"
	Event2 = "event2"
)

type TestMessages []struct {
	event string
	give  interface{}
	want  []byte
}

func genUrl(events []string, guid string, queryParams map[string]string) string {
	var params []string
	for _, event := range events {
		params = append(params, fmt.Sprintf("events=%s", event))
	}
	params = append(params, fmt.Sprintf("clientId=%s", uuid.New().String()))
	if queryParams != nil {
		for k, v := range queryParams {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return fmt.Sprintf("http://localhost:%d%s?%s", PORT, strings.ReplaceAll(TestPath, "{guid}", guid),
		strings.Join(params, "&"))
}

func _assertClientReceiveMessage(c C, guid string, event string, expectedMessages TestMessages, queryParams map[string]string) chan bool {
	done := make(chan bool)
	url := genUrl([]string{event}, guid, queryParams)
	client := sse.NewClient(url)

	go func() {
		idx := 0
		client.SubscribeRaw(func(e *sse.Event) {
			c.So(string(e.Event), ShouldEqual, event)
			c.So(e.Data, ShouldResemble, expectedMessages[idx].want)
			idx++

			if idx == len(expectedMessages) {
				done <- true
			}
		})
	}()

	return done
}

func assertClientReceiveMessageWithQueryParams(c C, guid string, event string, expectedMessages TestMessages,
	queryParams map[string]string) chan bool {
	return _assertClientReceiveMessage(c, guid, event, expectedMessages, queryParams)
}

func assertClientReceiveMessage(c C, guid string, event string, expectedMessages TestMessages) chan bool {
	return _assertClientReceiveMessage(c, guid, event, expectedMessages, nil)
}

func assertClientReceiveMessageWithMutipleEvents(c C, guid string, events []string, expectedMessages TestMessages) chan bool {
	done := make(chan bool)
	url := genUrl(events, guid, nil)
	client := sse.NewClient(url)

	go func() {
		idx := 0
		client.SubscribeRaw(func(e *sse.Event) {
			c.So(string(e.Event), ShouldEqual, expectedMessages[idx].event)
			c.So(e.Data, ShouldResemble, expectedMessages[idx].want)
			idx++

			if idx == len(expectedMessages) {
				done <- true
			}
		})
	}()

	return done
}

var (
	testSSEIO     SSEIO
	eventHandler1 EventHandler
	eventHandler2 EventHandler
	httpAddr      = flag.String("http.addr", fmt.Sprintf(":%d", PORT), "HTTP listen address")
)

func closeSSEIO() {
	// force to close
	testSSEIO.(*sseio).server.Close()
}

func initAttacheToHttpServerMode() {
	testSSEIO = NewSSEIO()
	r := mux.NewRouter()
	r.Handle(TestPath, testSSEIO).Methods("GET")
	r.HandleFunc("/hello", func(resp http.ResponseWriter, request *http.Request) {
		fmt.Fprint(resp, "hello world")
	})

	go func() {
		http.ListenAndServe(*httpAddr, r)
	}()
}

func initStandAloneMode() {
	testSSEIO = NewSSEIO(SetPath(TestPath), EnableEvent())
	go func() {
		for event := range testSSEIO.Events() {
			fmt.Println(event)
		}
	}()

	go func() {
		testSSEIO.Listen(*httpAddr)
	}()
}

func TestSSEIO(t *testing.T) {
	Convey("start standalone sseio server", t, func() {
		initStandAloneMode()

		eventHandler1, _ = testSSEIO.RegisterEventHandler(Event1,
			SetGetRoomIdFunc(func(context Context) string {
				return context.Params["guid"]
			}))
		eventHandler2, _ = testSSEIO.RegisterEventHandler(Event2,
			SetGetRoomIdFunc(func(context Context) string {
				return context.Params["guid"]
			}),
			SetFetchFunc(func(context Context) interface{} {
				query := context.Query
				if len(query["fetch"]) != 0 {
					return "fetch message"
				}

				return nil
			}))
	})

	Convey("in standalone mode", t, func() {
		Convey("single event", func() {
			Convey("should send message in any type", func(c C) {
				guid := uuid.New().String()
				giveTestStructMessage := struct {
					name string
					age  int
				}{
					"fooo",
					18,
				}
				wantTestStructMessage, _ := json.Marshal(giveTestStructMessage)
				giveTestMapMessage := map[string]interface{}{"name": "foo", "age": 18}
				wantTestMapMessage, _ := json.Marshal(giveTestMapMessage)

				messages := TestMessages{
					{
						give: "hello sseio",
						want: []byte("hello sseio"),
					},
					{
						give: giveTestStructMessage,
						want: wantTestStructMessage,
					},
					{
						give: giveTestMapMessage,
						want: wantTestMapMessage,
					},
				}
				done := assertClientReceiveMessage(c, guid, Event1, messages)
				time.Sleep(time.Millisecond * 100)

				for _, msg := range messages {
					eventHandler1.SendMessage(guid, msg.give)
				}

				<-done
			})

			Convey("should send messages to multiple clients in the same room", func(c C) {
				guid := uuid.New().String()
				messages := TestMessages{
					{
						give: "hello sseio",
						want: []byte("hello sseio"),
					},
				}
				done1 := assertClientReceiveMessage(c, guid, Event1, messages)
				done2 := assertClientReceiveMessage(c, guid, Event1, messages)
				time.Sleep(time.Millisecond * 100)
				eventHandler1.SendMessage(guid, messages[0].give)
				<-done1
				<-done2
			})

			Convey("should send messages to multiple clients in different rooms", func(c C) {
				guid1 := uuid.New().String()
				guid2 := uuid.New().String()
				messages1 := TestMessages{
					{
						give: "hello sseio1",
						want: []byte("hello sseio1"),
					},
				}
				messages2 := TestMessages{
					{
						give: "hello sseio2",
						want: []byte("hello sseio2"),
					},
				}
				done1 := assertClientReceiveMessage(c, guid1, Event1, messages1)
				done2 := assertClientReceiveMessage(c, guid2, Event1, messages2)
				time.Sleep(time.Millisecond * 100)
				eventHandler1.SendMessage(guid1, messages1[0].give)
				eventHandler1.SendMessage(guid2, messages2[0].give)
				<-done1
				<-done2
			})

			Convey("should send fetch messages", func(c C) {
				guid := uuid.New().String()
				done := assertClientReceiveMessageWithQueryParams(c, guid, Event2, TestMessages{
					{
						want: []byte("fetch message"),
					},
					{
						want: []byte("hello sseio"),
					},
				}, map[string]string{
					"fetch": "1",
				})
				time.Sleep(time.Millisecond * 100)
				eventHandler2.SendMessage(guid, "hello sseio")
				<-done
			})
		})

		Convey("multiple events", func() {
			Convey("should send message for client with two events", func(c C) {
				guid := uuid.New().String()
				messages := TestMessages{
					{
						event: Event1,
						give:  "event1 message",
						want:  []byte("event1 message"),
					},
					{
						event: Event2,
						give:  "event2 message",
						want:  []byte("event2 message"),
					},
				}
				done := assertClientReceiveMessageWithMutipleEvents(c, guid, []string{Event1, Event2}, messages)
				time.Sleep(time.Millisecond * 100)

				eventHandler1.SendMessage(guid, messages[0].give)
				eventHandler2.SendMessage(guid, messages[1].give)

				<-done
			})
		})
	})

	Convey("attaches to a http server", t, func() {
		closeSSEIO()
		initAttacheToHttpServerMode()

		eventHandler1, _ = testSSEIO.RegisterEventHandler(Event1,
			SetGetRoomIdFunc(func(context Context) string {
				return context.Params["guid"]
			}))
	})

	Convey("in attachment mode", t, func() {
		Convey("can http server also handle requests expect for sse", func() {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/hello", PORT))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			So(string(bodyBytes), ShouldEqual, "hello world")
		})

		Convey("can handle sse message as well", func(c C) {
			guid := uuid.New().String()
			messages := TestMessages{
				{
					give: "hello sseio",
					want: []byte("hello sseio"),
				},
			}
			done := assertClientReceiveMessage(c, guid, Event1, messages)
			time.Sleep(time.Millisecond * 100)

			for _, msg := range messages {
				eventHandler1.SendMessage(guid, msg.give)
			}

			<-done
		})
	})
}
