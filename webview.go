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

	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{}

type WebViewer struct {
	InputChan   chan Stat
	Addr        string
	srv         *http.Server
	data        []Stat
	outputChans []chan Stat
	mutex       *sync.Mutex
}

// NewWebViewer creates a new WebViewer that starts its own HTTP server
func NewWebViewer(inputChan chan Stat, addr string) *WebViewer {
	wv := &WebViewer{
		InputChan:   inputChan,
		Addr:        addr,
		data:        make([]Stat, 0),
		outputChans: make([]chan Stat, 0),
		mutex:       &sync.Mutex{},
	}
	wv.startOwnServer()
	wv.startDataProcessor()
	return wv
}

// NewWebViewerHandler creates a new WebViewer and registers handlers to the provided http.Handler
// This allows integration with any HTTP router/mux that implements http.Handler
func NewWebViewerHandler(inputChan chan Stat, handler http.Handler) *WebViewer {
	wv := &WebViewer{
		InputChan:   inputChan,
		data:        make([]Stat, 0),
		outputChans: make([]chan Stat, 0),
		mutex:       &sync.Mutex{},
	}
	wv.registerToHandler(handler)
	wv.startDataProcessor()
	return wv
}

func (wv *WebViewer) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	content, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Could not load index page", http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(content))
}

func (wv *WebViewer) serveStatic(w http.ResponseWriter, r *http.Request) {
	// Extract filename from URL path
	path := strings.TrimPrefix(r.URL.Path, "/ptest/static/")
	filename := strings.Split(path, "/")[0]

	// Set appropriate content type
	if strings.HasSuffix(filename, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(filename, ".css") {
		w.Header().Set("Content-Type", "text/css")
	}

	content, err := staticFiles.ReadFile("static/" + filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Write(content)
}

func (wv *WebViewer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
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

// registerToHandler registers ptest handlers to any http.Handler that supports route registration
// This works with standard http.ServeMux, gorilla/mux, gin, echo, etc.
func (wv *WebViewer) registerToHandler(handler http.Handler) {
	// For standard http.ServeMux
	if mux, ok := handler.(*http.ServeMux); ok {
		mux.HandleFunc("/ptest/", wv.serveIndex)
		mux.HandleFunc("/ptest/static/", wv.serveStatic)
		mux.HandleFunc("/ptest/ws", wv.handleWebSocket)
		return
	}

	// For other routers, we need a more generic approach
	// This might require type assertion for specific router types
	// or we could provide a separate method for manual registration
	log.Println("Handler registration: please manually register handlers using GetHandler() method")
}

// GetHandler returns an http.Handler that can be mounted at /ptest prefix
func (wv *WebViewer) GetHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", wv.serveIndex)
	mux.HandleFunc("/static/", wv.serveStatic)
	mux.HandleFunc("/ws", wv.handleWebSocket)
	return mux
}

func (wv *WebViewer) startOwnServer() {
	mux := http.NewServeMux()
	mux.Handle("/ptest/", http.StripPrefix("/ptest", wv.GetHandler()))

	wv.srv = &http.Server{
		Addr:         wv.Addr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      mux,
	}

	go func() {
		if err := wv.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println(err)
		}
	}()

	// Log the server address for user convenience
	addr := wv.Addr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	log.Printf("WebViewer started at http://%s/ptest", addr)
}

func (wv *WebViewer) startDataProcessor() {
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

				if wv.srv != nil {
					wv.srv.Close()
				}
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
