package ptest

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin
	},
}

// WebViewer handles web interface for TestRunner
type WebViewer struct {
	testRunner *TestRunner
	srv        *http.Server
	clients    map[*websocket.Conn]bool
	mutex      sync.RWMutex
}

// newWebViewer creates a WebViewer with its own server
func newWebViewer(testRunner *TestRunner, addr string, ownServer bool) *WebViewer {
	wv := &WebViewer{
		testRunner: testRunner,
		clients:    make(map[*websocket.Conn]bool),
		mutex:      sync.RWMutex{},
	}

	if ownServer {
		wv.startOwnServer(addr)
	}

	return wv
}

// newWebViewerWithHandler creates a WebViewer with handler registration
func newWebViewerWithHandler(testRunner *TestRunner, registrar HandlerRegistrar) *WebViewer {
	wv := &WebViewer{
		testRunner: testRunner,
		clients:    make(map[*websocket.Conn]bool),
		mutex:      sync.RWMutex{},
	}

	wv.registerHandlers(registrar)
	return wv
}

// startOwnServer starts the web viewer's own HTTP server
func (wv *WebViewer) startOwnServer(addr string) {
	mux := http.NewServeMux()
	wv.registerHandlers(mux)

	wv.srv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	go func() {
		if err := wv.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("WebViewer server error: %v", err)
		}
	}()

	// Log server start
	displayAddr := addr
	if strings.HasPrefix(addr, ":") {
		displayAddr = "localhost" + addr
	}
	log.Printf("Performance test dashboard available at http://%s/ptest/", displayAddr)
}

// registerHandlers registers HTTP handlers
func (wv *WebViewer) registerHandlers(registrar HandlerRegistrar) {
	registrar.HandleFunc("/ptest/", wv.serveIndex)
	registrar.HandleFunc("/ptest/static/", wv.serveStatic)
	registrar.HandleFunc("/ptest/ws", wv.handleWebSocket)
	registrar.HandleFunc("/ptest/api/sessions", wv.handleSessions)
	registrar.HandleFunc("/ptest/api/current", wv.handleCurrentSession)
}

// serveIndex serves the main HTML page
func (wv *WebViewer) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	content, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		log.Printf("Failed to read index.html: %v", err)
		http.Error(w, "Could not load index page", http.StatusInternalServerError)
		return
	}
	w.Write(content)
}

// serveStatic serves static files
func (wv *WebViewer) serveStatic(w http.ResponseWriter, r *http.Request) {
	var filename string
	if strings.HasPrefix(r.URL.Path, "/ptest/static/") {
		filename = strings.TrimPrefix(r.URL.Path, "/ptest/static/")
	} else {
		http.NotFound(w, r)
		return
	}

	if filename == "" {
		http.NotFound(w, r)
		return
	}

	// Set content type
	if strings.HasSuffix(filename, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(filename, ".css") {
		w.Header().Set("Content-Type", "text/css")
	}

	content, err := staticFiles.ReadFile("static/" + filename)
	if err != nil {
		log.Printf("Failed to read static file %s: %v", filename, err)
		http.NotFound(w, r)
		return
	}

	w.Write(content)
}

// handleSessions returns all sessions
func (wv *WebViewer) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions := wv.testRunner.ListSessions()

	var sessionList []*SessionStats
	for _, session := range sessions {
		sessionList = append(sessionList, session.GetStats())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionList)
}

// handleCurrentSession returns current session info
func (wv *WebViewer) handleCurrentSession(w http.ResponseWriter, r *http.Request) {
	currentSession := wv.testRunner.GetCurrentSession()
	if currentSession == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
		return
	}

	stats := currentSession.GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleWebSocket handles WebSocket connections
func (wv *WebViewer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Add client
	wv.mutex.Lock()
	wv.clients[conn] = true
	wv.mutex.Unlock()

	// Remove client on disconnect
	defer func() {
		wv.mutex.Lock()
		delete(wv.clients, conn)
		wv.mutex.Unlock()
	}()

	// Send initial data if there's a current session
	if currentSession := wv.testRunner.GetCurrentSession(); currentSession != nil {
		wv.sendToClient(conn, WSMessage{
			Type:      MsgTypeSessionStart,
			SessionID: currentSession.ID,
			Data:      currentSession.GetStats(),
		})

		// Send current chart data
		chartData := currentSession.GetOptimizedChartData()
		wv.sendToClient(conn, WSMessage{
			Type: MsgTypeOptimizedData,
			Data: chartData,
		})
	}

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// sendToClient sends a message to a specific client
func (wv *WebViewer) sendToClient(conn *websocket.Conn, message WSMessage) {
	if err := conn.WriteJSON(message); err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
}

// broadcast sends a message to all connected clients
func (wv *WebViewer) broadcast(message WSMessage) {
	wv.mutex.RLock()
	defer wv.mutex.RUnlock()

	for client := range wv.clients {
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Broadcast error: %v", err)
			client.Close()
			delete(wv.clients, client)
		}
	}
}

// onSessionStart notifies about session start
func (wv *WebViewer) onSessionStart(session *TestSession) {
	wv.broadcast(WSMessage{
		Type:      MsgTypeSessionStart,
		SessionID: session.ID,
		Data:      session.GetStats(),
	})

	// Start sending real-time data
	go wv.streamSessionData(session)
}

// onSessionStop notifies about session stop
func (wv *WebViewer) onSessionStop(session *TestSession) {
	wv.broadcast(WSMessage{
		Type:      MsgTypeSessionStop,
		SessionID: session.ID,
		Data:      session.GetStats(),
	})
}

// streamSessionData streams real-time data for a session
func (wv *WebViewer) streamSessionData(session *TestSession) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if session.Status != StatusRunning {
			break
		}

		// Send both chart data and session stats
		chartData := session.GetOptimizedChartData()
		sessionStats := session.GetStats()

		wv.broadcast(WSMessage{
			Type: MsgTypeOptimizedData,
			Data: map[string]interface{}{
				"chart_data":    chartData,
				"session_stats": sessionStats,
			},
		})
	}
}

// Close gracefully shuts down the web viewer
func (wv *WebViewer) Close() error {
	// Close all WebSocket connections
	wv.mutex.Lock()
	for client := range wv.clients {
		client.Close()
	}
	wv.mutex.Unlock()

	// Close HTTP server if we own it
	if wv.srv != nil {
		return wv.srv.Close()
	}

	return nil
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// WebSocket message types
const (
	MsgTypeData          = "data"
	MsgTypeReset         = "reset"
	MsgTypeSessionStart  = "session_start"
	MsgTypeSessionStop   = "session_stop"
	MsgTypeOptimizedData = "optimized_data"
)
