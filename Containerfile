# --------------------------------------------
# 1) Builder Stage
# --------------------------------------------
    FROM registry.access.redhat.com/ubi8/go-toolset:1.19 AS builder

    # Switch to a known writable directory for non-root
    WORKDIR /opt/app-root/src
    
    # Copy your code from cmd/k8s-concepts-demo to the working directory
    COPY cmd/container-concepts-demo/ . 
    
    # Build the Go binary
    # (Adjust the file name(s) if main.go has a different name)
    RUN go build -o container-concepts-demo main.go
    
    
    # --------------------------------------------
    # 2) Final Runtime Stage
    # --------------------------------------------
    FROM registry.access.redhat.com/ubi8-minimal:latest
    
    # (Optional) install shadow-utils so we can add a named non-root user
    RUN microdnf install -y shadow-utils && microdnf clean all
    
    # Create a non-root user (recommended in OpenShift)
    RUN useradd -u 1001 -r -g root k8suser
    USER 1001
    
    # Copy the compiled binary from builder
    COPY --from=builder /opt/app-root/src/container-concepts-demo /usr/local/bin/container-concepts-demo
    
    # Expose the port your app listens on
    EXPOSE 3000
    
    # Default env or override at runtime
    ENV APP_NAME=container-concepts-demo
    
    # Start the app
    ENTRYPOINT ["/usr/local/bin/container-concepts-demo"]
    