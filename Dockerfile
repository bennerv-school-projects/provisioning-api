FROM golang:alpine as BUILDER

MAINTAINER Ben Vesel "bves94@gmail.com"

# Copy Jar File over
COPY main /

# Expose Port 8080
EXPOSE 8080

# Run command
ENTRYPOINT ["/main"]
