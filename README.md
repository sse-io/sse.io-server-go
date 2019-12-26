# SSE-IO For Golang

Golang server for SSE-IO.

## Install

```shell
go get github.com/sse-io/sse.io-server-go
```

## How to use

### Standalone server

```go
httpAddr := flag.String("http.addr", fmt.Sprintf(":%d", 9001), "HTTP listen address")
path := "/files/:guid/pull"
server := sseio.NewSSEIO(sseio.SetPath(path))

eventHandler, _ := server.RegisterEventHandler("event",
    sseio.SetGetRoomIdFunc(func(context sseio.Context) string {
        return context.Params["guid"]
    }),
)

go func() {
    server.Listen(*httpAddr)
}()

eventHandler.SendMessage("guid1", "hello sseio")
```

### attaching to your http server

you should install **"github.com/gorilla/mux"** first.

```go
path := "/files/:guid/pull"
server := sseio.NewSSEIO()

// ... regist event handler

r := mux.NewRouter()
r.Handle(path, server).Methods("GET")
r.HandleFunc("/hello", func(resp http.ResponseWriter, request *http.Request) {
    fmt.Fprint(resp, "hello world")
})

go func() {
    http.ListenAndServe(*httpAddr, r)
}()
```