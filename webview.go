package ptest

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{}

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	content, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Could not load index page", http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(content))
}

type WebViewer struct {
	InputChan   chan Stat
	Addr        string
	srv         *http.Server
	data        []Stat
	outputChans []chan Stat
	mutex       *sync.Mutex
}

func NewWebViewer(inputChan chan Stat, addr string) *WebViewer {
	wv := &WebViewer{
		InputChan:   inputChan,
		Addr:        addr,
		data:        make([]Stat, 0),
		outputChans: make([]chan Stat, 0),
		mutex:       &sync.Mutex{},
	}
	wv.start()
	return wv
}

func (wv *WebViewer) GenWebSocketHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		outputChan := make(chan Stat, 100)
		wv.addOutputChan(outputChan)
		defer wv.removeOutputChan(outputChan)

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		defer ws.Close()

		for {
			sr := <-outputChan

			b, err := json.Marshal(sr)
			if err != nil {
				log.Println("marshal:", err)
				break
			}

			err = ws.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				log.Println("write:", err)
				break
			}
		}
		log.Println("wv end")
	}
}

func (wv *WebViewer) addOutputChan(c chan Stat) {
	wv.mutex.Lock()
	defer wv.mutex.Unlock()
	wv.outputChans = append(wv.outputChans, c)
	go func() {
		for _, d := range wv.data {
			c <- d
		}
	}()
}

func (wv *WebViewer) removeOutputChan(c chan Stat) {
	wv.mutex.Lock()
	defer wv.mutex.Unlock()

	last := len(wv.outputChans) - 1
	for i := range wv.outputChans {
		if wv.outputChans[i] == c {
			wv.outputChans[i], wv.outputChans[last] = wv.outputChans[last], wv.outputChans[i]
			wv.outputChans = wv.outputChans[:last]
			close(c)
			return
		}
	}
}

func (wv *WebViewer) start() {
	r := mux.NewRouter()
	r.HandleFunc("/", serveHome)
	r.HandleFunc("/ws", wv.GenWebSocketHandler())

	wv.srv = &http.Server{
		Addr:         wv.Addr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	go func() {
		if err := wv.srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	// Log the server address for user convenience
	addr := wv.Addr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	log.Printf("WebViewer started at http://%s", addr)

	go func() {
		for {
			stat, more := <-wv.InputChan
			if !more {
				fmt.Println("webview:no more input")

				wv.mutex.Lock()
				for _, c := range wv.outputChans {
					close(c)
				}
				wv.outputChans = nil
				wv.mutex.Unlock()

				wv.srv.Close()
				return
			}

			wv.mutex.Lock()
			wv.data = append(wv.data, stat)
			for _, outputChan := range wv.outputChans {
				outputChan <- stat
			}
			wv.mutex.Unlock()
		}
	}()
}
