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
	log.Println("init")

	// Collector collects raw data
	collector := ptest.NewCollector()
	// Monitor processes raw data to stat
	m := ptest.NewMonitor(collector.ResultChan)

	// Create HTTP server mux
	mux := http.NewServeMux()

	// Create WebViewer handler (no own server)
	ptest.NewWebViewerHandler(m.ResultChan, mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":9090",
		Handler:      mux,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Custom HTTP server started at http://localhost:9090")
		log.Printf("Performance test dashboard available at http://localhost:9090/ptest/")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Println("start performance test")
	stop := false
	var wg sync.WaitGroup

	// Simulate performance test workload
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop {
				start := time.Now()
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(99)))
				result := true
				if rand.Intn(10) < 2 {
					result = false
				}
				// Report test result
				collector.Report(start, result)
			}
		}()
	}

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	log.Println("Shutting down...")
	stop = true

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	wg.Wait()
	log.Println("goroutines - stopped")

	// Stop data collection chain
	collector.Stop()
	time.Sleep(time.Second)
	log.Println("Application stopped")
}
