package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/realcoke/ptest"
)

func main() {
	log.Println("Initializing Custom HTTP Server with Performance Test...")

	// Create HTTP server mux
	mux := http.NewServeMux()

	// Add your own endpoints
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello! Performance dashboard is at /ptest/"))
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create TestRunner with handler registration (no own server)
	runner := ptest.NewTestRunnerWithHandler(mux)
	defer runner.Close()

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":9090",
		Handler:      mux,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	// Start server
	go func() {
		log.Printf("Custom HTTP server started at http://localhost:9090")
		log.Printf("Performance test dashboard available at http://localhost:9090/ptest/")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start performance test
	log.Println("Starting performance test session...")
	runner.StartTest("Custom Server Load Test")

	// Simulate test workload
	stop := false
	var wg sync.WaitGroup

	// Simulate 100 concurrent users with varying load
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			for !stop {
				start := time.Now()

				// Simulate different types of operations with different durations
				operationType := rand.Intn(3)
				var workDuration time.Duration
				var successRate int

				switch operationType {
				case 0: // Fast operation
					workDuration = time.Duration(rand.Intn(50)+10) * time.Millisecond
					successRate = 95
				case 1: // Medium operation
					workDuration = time.Duration(rand.Intn(100)+50) * time.Millisecond
					successRate = 90
				case 2: // Slow operation
					workDuration = time.Duration(rand.Intn(200)+100) * time.Millisecond
					successRate = 85
				}

				time.Sleep(workDuration)

				success := rand.Intn(100) < successRate
				runner.Report(start, success)

				// Random think time between requests
				thinkTime := time.Duration(rand.Intn(1000)) * time.Millisecond
				time.Sleep(thinkTime)
			}
		}(i)
	}

	log.Printf("Performance test running with 100 concurrent users")
	log.Println("Press Ctrl+C to stop...")

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	log.Println("Shutting down...")
	stop = true
	runner.StopTest()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	wg.Wait()
	log.Println("Application stopped")
}
