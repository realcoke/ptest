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

// HandlerRegistrar is an interface for types that can register HTTP handlers
type HandlerRegistrar interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

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

// NewWebViewerHandler creates a new WebViewer and registers handlers to the provided handler
func NewWebViewerHandler(inputChan chan Stat, registrar HandlerRegistrar) *WebViewer {
	wv := &WebViewer{
		InputChan:   inputChan,
		data:        make([]Stat, 0),
		outputChans: make([]chan Stat, 0),
		mutex:       &sync.Mutex{},
	}
	wv.registerHandlers(registrar)
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
	// Extract filename from URL path - handle both /ptest/static/ and /static/ patterns
	var filename string
	if strings.HasPrefix(r.URL.Path, "/ptest/static/") {
		filename = strings.TrimPrefix(r.URL.Path, "/ptest/static/")
	} else if strings.HasPrefix(r.URL.Path, "/static/") {
		filename = strings.TrimPrefix(r.URL.Path, "/static/")
	} else {
		http.NotFound(w, r)
		return
	}

	// Remove any additional path segments, keep only the filename
	if idx := strings.Index(filename, "/"); idx != -1 {
		filename = filename[:idx]
	}

	if filename == "" {
		http.NotFound(w, r)
		return
	}

	// Set appropriate content type
	if strings.HasSuffix(filename, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(filename, ".css") {
		w.Header().Set("Content-Type", "text/css")
	}

	content, err := staticFiles.ReadFile("static/" + filename)
	if err != nil {
		log.Printf("Failed to read static file: %s, error: %v", filename, err)
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

// registerHandlers registers ptest handlers to any HandlerRegistrar
func (wv *WebViewer) registerHandlers(registrar HandlerRegistrar) {
	registrar.HandleFunc("/ptest/", wv.serveIndex)
	registrar.HandleFunc("/ptest/static/", wv.serveStatic)
	registrar.HandleFunc("/ptest/ws", wv.handleWebSocket)
}

func (wv *WebViewer) startOwnServer() {
	mux := http.NewServeMux()
	// 중복 제거: registerHandlers 사용
	wv.registerHandlers(mux)

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
	log.Printf("WebViewer started at http://%s/ptest/", addr)
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
