package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Global flags for toggling liveness/readiness
var (
	isAlive = true
	isReady = true
)

// Environment-based variables
var (
	appName       = getEnv("APP_NAME", "container-concepts-demo")
	hostname      string
	shutdownDelay = getEnvAsInt("SHUTDOWN_DELAY", 3)
	// UNREADY_ON_SHUTDOWN now defaults to true:
	unreadyOnShutdown = getEnvAsBool("UNREADY_ON_SHUTDOWN", true)
	// If UNREADY_ON_SHUTDOWN=true => we set isReady=false as soon as we receive SIGTERM.
	// If SHUTDOWN_DELAY=-1 => never shut down (simulate stuck).
)

// Memory hog variables
var (
	memoryHog [][]byte
	hogMu     sync.Mutex

	hogging bool
	hogStop chan struct{}
)

func main() {
	hostname, _ := os.Hostname()

	// ---- Log the current variables at startup ----
	log.Println("=================================================")
	log.Printf("Starting up with the following settings:")
	log.Printf("APP_NAME            = %s", appName)
	log.Printf("HOSTNAME            = %s", hostname)
	log.Printf("SHUTDOWN_DELAY      = %d", shutdownDelay)
	log.Printf("UNREADY_ON_SHUTDOWN = %v", unreadyOnShutdown)
	log.Println("=================================================")

	mux := http.NewServeMux()

	// Liveness endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request to /healthz from %s\n", r.RemoteAddr)
		if isAlive {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ALIVE")
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "NOT ALIVE")
		}
	})

	// Readiness endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request to /ready from %s\n", r.RemoteAddr)
		if isReady {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "READY")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "NOT READY")
		}
	})

	// Toggle liveness
	mux.HandleFunc("/toggle-alive", func(w http.ResponseWriter, r *http.Request) {
		isAlive = !isAlive
		log.Printf("Toggled isAlive to %v\n", isAlive)
		fmt.Fprintf(w, "Liveness is now: %v for Pod %s\n", isAlive, hostname)
	})

	// Toggle readiness
	mux.HandleFunc("/toggle-ready", func(w http.ResponseWriter, r *http.Request) {
		isReady = !isReady
		log.Printf("Toggled isReady to %v\n", isReady)
		fmt.Fprintf(w, "Readiness is now: %v for Pod %s\n", isReady, hostname)
	})

	// Memory hogging features
	// Start auto-hog
	mux.HandleFunc("/start-hog", startHogHandler)
	// Stop auto-hog
	mux.HandleFunc("/stop-hog", stopHogHandler)
	// Clear all allocated memory
	mux.HandleFunc("/reset-hog", resetHogHandler)
	// One-shot hog
	mux.HandleFunc("/hog", oneShotHogHandler)

	// Root endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request to / from %s\n", r.RemoteAddr)
		fmt.Fprintf(w, "Hello from %s on Pod %s\n", appName, hostname)
	})

	server := &http.Server{
		Addr:    ":3000",
		Handler: mux,
	}

	// Listen for shutdown signals
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	// Run server in a goroutine
	go func() {
		log.Printf("Starting HTTP server at :3000...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v\n", err)
		}
	}()

	// Block until we get a signal
	sig := <-stopChan
	log.Printf("Received signal: %v. Starting graceful shutdown logic...\n", sig)

	// If possible, show numeric signal
	if s, ok := sig.(syscall.Signal); ok {
		sigDesc := describeSignal(s)
		log.Printf("Signal details => number %d (%s)", s, sigDesc)
	}

	// Optionally set NOT READY on shutdown to stop receiving traffic
	if unreadyOnShutdown {
		log.Println("UNREADY_ON_SHUTDOWN=true => setting isReady=false now.")
		isReady = false
	}

	// If SHUTDOWN_DELAY is -1, simulate a stuck container
	if shutdownDelay < 0 {
		log.Println("SHUTDOWN_DELAY = -1 => Simulating a stuck pod. Blocking forever...")
		select {} // never exit
	}

	// Otherwise, sleep for the configured delay
	log.Printf("Sleeping for %d second(s) before shutting down...\n", shutdownDelay)
	time.Sleep(time.Duration(shutdownDelay) * time.Second)

	// Attempt graceful shutdown with 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v\n", err)
	}

	log.Println("Cleanup complete. Exiting.")
}

// -----------------------------------------------------
// Memory Hog Handlers
// -----------------------------------------------------

func startHogHandler(w http.ResponseWriter, r *http.Request) {
	mbParam := r.URL.Query().Get("mb")
	if mbParam == "" {
		mbParam = "5" // default 5 MiB
	}
	mb, err := strconv.Atoi(mbParam)
	if err != nil || mb <= 0 {
		http.Error(w, "Invalid mb parameter. Must be > 0.", http.StatusBadRequest)
		return
	}

	if hogging {
		fmt.Fprintf(w, "Already hogging memory on pod %s. Stop first or keep going.\n", hostname)
		return
	}

	startHog(mb)
	fmt.Fprintf(w, "Started allocating %d MiB per second on pod %s.\n", mb, hostname)
}

func stopHogHandler(w http.ResponseWriter, r *http.Request) {
	if hogging {
		stopHog()
		fmt.Fprintf(w, "Stopped hogging memory on pod %s.\n", hostname)
	} else {
		fmt.Fprintf(w, "Not currently hogging on pod %s.\n", hostname)
	}
}

func resetHogHandler(w http.ResponseWriter, r *http.Request) {
	hogMu.Lock()
	memoryHog = nil
	hogMu.Unlock()

	fmt.Fprintf(w, "Memory allocations reset on pod %s. (Chunks cleared.)", hostname)
}

func oneShotHogHandler(w http.ResponseWriter, r *http.Request) {
	mbParam := r.URL.Query().Get("mb")
	if mbParam == "" {
		mbParam = "10"
	}
	mb, err := strconv.Atoi(mbParam)
	if err != nil || mb <= 0 {
		http.Error(w, "Invalid mb parameter. Must be > 0.", http.StatusBadRequest)
		return
	}

	hogMu.Lock()
	for i := 0; i < mb; i++ {
		chunk := make([]byte, 1_000_000)
		for j := range chunk {
			chunk[j] = 1
		}
		memoryHog = append(memoryHog, chunk)
	}
	total := len(memoryHog)
	hogMu.Unlock()

	msg := fmt.Sprintf("Allocated %d MiB in one shot on pod %s. Total chunks: %d\n", mb, hostname, total)
	log.Println(msg)
	fmt.Fprint(w, msg)
}

// -----------------------------------------------------
// Background Memory Hog Logic
// -----------------------------------------------------

func startHog(mb int) {
	hogStop = make(chan struct{})
	hogging = true

	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Allocate 'mb' MiB
				hogMu.Lock()
				for i := 0; i < mb; i++ {
					chunk := make([]byte, 1_000_000)
					for j := range chunk {
						chunk[j] = 1
					}
					memoryHog = append(memoryHog, chunk)
				}
				total := len(memoryHog)
				hogMu.Unlock()
				log.Printf("Allocated %d MiB this tick. Total chunks: %d\n", mb, total)

			case <-hogStop:
				hogMu.Lock()
				hogging = false
				hogMu.Unlock()

				log.Println("Stopped hogging.")
				return
			}
		}
	}()
}

func stopHog() {
	close(hogStop)
}

// describeSignal translates known signals to names
func describeSignal(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	default:
		return "unknown"
	}
}

// getEnv reads an env var with a fallback
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

// getEnvAsInt reads an env var as an integer (with fallback)
func getEnvAsInt(key string, fallback int) int {
	valStr := getEnv(key, "")
	if valStr == "" {
		return fallback
	}
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return fallback
}

// getEnvAsBool reads an env var as a boolean (with a default fallback)
func getEnvAsBool(key string, fallback bool) bool {
	valStr := getEnv(key, "")
	if valStr == "" {
		return fallback
	}
	switch valStr {
	case "true", "1", "yes", "TRUE", "True":
		return true
	case "false", "0", "no", "FALSE", "False":
		return false
	default:
		return fallback
	}
}
