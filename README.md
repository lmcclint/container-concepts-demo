# container-concepts-demo

## Usage
    oc apply -k https://github.com/lmcclint/container-concepts-demo/deploy

Creates a 3 pod deployment in the namespace `container-concepts-demo` capable of demonstrating a number of container features

## Features

### Liveness & Readiness

Demonstrates how failing liveness triggers a pod restart, while failing readiness removes the pod from service endpoints.

    Endpoints:
        GET /healthz → liveness check (returns 200 ALIVE or 500 NOT ALIVE)
        GET /ready → readiness check (returns 200 READY or 503 NOT READY)

    Toggle Endpoints:
        GET /toggle-alive → toggles the liveness state
        GET /toggle-ready → toggles the readiness state


### Graceful Shutdown & Signal Handling

Demonstrates how a pod can stop receiving new traffic while finishing inflight requests, or simulates various timeout conditions.

    * Listens for SIGTERM (typical signal on pod shutdown).
    * Logs both signal number and description (SIGTERM, SIGINT, etc.).
    * Waits a configurable number of seconds before fully exiting (via SHUTDOWN_DELAY env variable).
        Set SHUTDOWN_DELAY to -1 to simulate a stuck container that never completes shutdown (K8s eventually kills it after terminationGracePeriodSeconds).
    * Configurable option to toggle readiness check during SHUTDOWN_DELAY (default: true) 
        Control via UNREADY_ON_SHUTDOWN env var

### Memory Hog / OOM Demo

Demonstrates how OpenShift memory limits lead to OOM kills. Deployment default limit is 64MiB

    * Multiple endpoints to demonstrate various memory consumption and problem behaviors: 
        /start-hog?mb=<N>: Allocates <N> MB every second in a loop.
        /stop-hog: Stops the memory allocation loop.
        /hog?mb=<N>: One-shot allocation of <N> MB. (default 10Mi)
        /reset-hog: Clears all allocations, allowing garbage collection to (eventually) free the memory.


### Environment Variable Configuration

`APP_NAME`: Name displayed in logs/responses (default: container-concepts-demo).

`SHUTDOWN_DELAY`: Seconds to wait after SIGTERM before exiting (default: 3). Set -1 to never shutdown. 

`UNREADY_ON_SHUTDOWN`: Whether to set readiness to false on shutdown (default: true).

### Logging 
Logs every request to stdout (both toggle endpoints and memory hog).

Prints startup configuration (e.g., APP_NAME, SHUTDOWN_DELAY) for transparency.

Useful for understanding container behavior in a live cluster