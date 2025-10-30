package main

import (
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/realcoke/ptest"
)

func main() {
	log.Println("Initializing Performance Test Runner...")

	// Create TestRunner with its own server
	runner := ptest.NewTestRunner(":9090")
	defer runner.Close()

	// Start performance test
	log.Println("Starting performance test session...")
	runner.StartTest("Simple Load Test")

	// Run test workload
	stop := false
	var wg sync.WaitGroup

	// Simulate 50 concurrent users
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			for !stop {
				// Simulate work
				start := time.Now()

				// Random work duration (10-100ms)
				workDuration := time.Duration(rand.Intn(90)+10) * time.Millisecond
				time.Sleep(workDuration)

				// Random success/failure (90% success rate)
				success := rand.Intn(100) < 90

				// Report test result
				runner.Report(start, success)
			}
		}(i)
	}

	log.Printf("Performance test running with 50 concurrent users")
	log.Printf("Dashboard available at: http://localhost:9090/ptest/")
	log.Println("Press Ctrl+C to stop...")

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	log.Println("Stopping test...")
	stop = true
	runner.StopTest()

	wg.Wait()
	log.Println("Test completed")
}
