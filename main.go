package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type Event struct {
	message    string
	channel_id string
}

type Broker struct {
	clients map[chan Event][]string
	mutex   *sync.Mutex
	counter int
}

func (b *Broker) Subscribe(client_channels []string) chan Event {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	c := make(chan Event)
	b.clients[c] = client_channels
	return c
}

func (b *Broker) Unsubscribe(c chan Event) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	close(c)
	delete(b.clients, c)
}

func (b *Broker) Publish(new Event) {
	// publish to clients on a channel
	// publish to all clients if channel is not specified
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for e, channelInfo := range b.clients {
		channel_id := channelInfo[1]
		if new.channel_id == "" || new.channel_id == channel_id {
			e <- new
		}
	}
}

func (b *Broker) Close() {
	for k, _ := range b.clients {
		close(k)
		delete(b.clients, k)
	}
}

func marshalJson(mapped_json map[string]string) []byte {
	jsonResp, err := json.Marshal(mapped_json)
	if err != nil {
		log.Panicf("Error marshalling response: %s", err)
	}
	return jsonResp
}

func returnErr(w http.ResponseWriter, message string, status_code int) {
	w.WriteHeader(http.StatusBadRequest)
	m := make(map[string]string)
	m["message"] = message
	resp := marshalJson(m)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
	return
}

func (broker *Broker) connectHandler(w http.ResponseWriter, r *http.Request) {
	// subscribe to a channel using userId
	userId := r.URL.Query().Get("userId")
	if userId == "" {
		m := "User ID is required"
		returnErr(w, m, http.StatusBadRequest)
		return
	}

	channels := []string{
		userId,
		userId, // set the channel to userId for now
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Cannot stream", http.StatusBadRequest)
		return
	}

	c := broker.Subscribe(channels)
	defer broker.Unsubscribe(c)

	fmt.Printf("%+v\n", broker)

	fmt.Fprintf(w, "data: %s\n\n", "Connected to server!")
	flusher.Flush()

	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			// send event message to client
			event := <-c
			fmt.Fprintf(w, "data: %v\n\n", event.message)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (broker *Broker) messageHandler(w http.ResponseWriter, r *http.Request) {
	// publish message to all clients on a channel
	// if no channel is specified, publish to all clients
	channelId := r.URL.Query().Get("channelId")
	if channelId == "" {
		channelId = ""
	}
	message := r.URL.Query().Get("message")
	fmt.Printf(r.URL.Path)
	e := Event{
		message:    message,
		channel_id: channelId,
	}
	broker.Publish(e)
}

func main() {
	broker := Broker{
		clients: make(map[chan Event][]string),
		mutex:   new(sync.Mutex),
	}
	http.HandleFunc("/connect", broker.connectHandler)
	http.HandleFunc("/message", broker.messageHandler)
	// Serve index.html when the user goes to /
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	fmt.Printf("Starting SSE server on port 8080 \n")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
