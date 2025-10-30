package ptest

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// HandlerRegistrar is an interface for types that can register HTTP handlers
type HandlerRegistrar interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// TestRunner manages test sessions and provides a unified interface
type TestRunner struct {
	sessions       map[string]*TestSession
	currentSession *TestSession
	webViewer      *WebViewer
	mutex          sync.RWMutex
	isOwnServer    bool
}

// NewTestRunner creates a TestRunner with its own HTTP server
func NewTestRunner(addr string) *TestRunner {
	tr := &TestRunner{
		sessions:    make(map[string]*TestSession),
		isOwnServer: true,
		mutex:       sync.RWMutex{},
	}

	tr.webViewer = newWebViewer(tr, addr, true)
	return tr
}

// NewTestRunnerWithHandler creates a TestRunner that registers handlers to existing server
func NewTestRunnerWithHandler(registrar HandlerRegistrar) *TestRunner {
	tr := &TestRunner{
		sessions:    make(map[string]*TestSession),
		isOwnServer: false,
		mutex:       sync.RWMutex{},
	}

	tr.webViewer = newWebViewerWithHandler(tr, registrar)
	return tr
}

// StartTest creates and starts a new test session
func (tr *TestRunner) StartTest(name string) *TestSession {
	tr.mutex.Lock()
	defer tr.mutex.Unlock()

	// Stop current session if running
	if tr.currentSession != nil && tr.currentSession.Status == StatusRunning {
		tr.currentSession.stop()
	}

	// Create new session
	sessionID := generateSessionID()
	session := newTestSession(sessionID, name)

	tr.sessions[sessionID] = session
	tr.currentSession = session

	// Start the session
	session.start()

	// Notify web viewer about new session
	tr.webViewer.onSessionStart(session)

	log.Printf("Started new test session: %s (%s)", name, sessionID)
	return session
}

// StopTest stops the current test session
func (tr *TestRunner) StopTest() {
	tr.mutex.Lock()
	defer tr.mutex.Unlock()

	if tr.currentSession != nil && tr.currentSession.Status == StatusRunning {
		tr.currentSession.stop()
		tr.webViewer.onSessionStop(tr.currentSession)
		log.Printf("Stopped test session: %s", tr.currentSession.Name)
	}
}

// Report reports a test result to the current session
func (tr *TestRunner) Report(start time.Time, success bool) {
	tr.mutex.RLock()
	session := tr.currentSession
	tr.mutex.RUnlock()

	if session != nil && session.Status == StatusRunning {
		session.Report(start, success)
	}
}

// GetCurrentSession returns the current active session
func (tr *TestRunner) GetCurrentSession() *TestSession {
	tr.mutex.RLock()
	defer tr.mutex.RUnlock()
	return tr.currentSession
}

// GetSession returns a session by ID
func (tr *TestRunner) GetSession(sessionID string) *TestSession {
	tr.mutex.RLock()
	defer tr.mutex.RUnlock()
	return tr.sessions[sessionID]
}

// ListSessions returns all sessions
func (tr *TestRunner) ListSessions() map[string]*TestSession {
	tr.mutex.RLock()
	defer tr.mutex.RUnlock()

	result := make(map[string]*TestSession)
	for k, v := range tr.sessions {
		result[k] = v
	}
	return result
}

// Close gracefully shuts down the test runner
func (tr *TestRunner) Close() error {
	tr.mutex.Lock()
	defer tr.mutex.Unlock()

	// Stop current session
	if tr.currentSession != nil {
		tr.currentSession.stop()
	}

	// Stop all sessions
	for _, session := range tr.sessions {
		session.stop()
	}

	// Close web viewer
	if tr.webViewer != nil {
		return tr.webViewer.Close()
	}

	return nil
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}
