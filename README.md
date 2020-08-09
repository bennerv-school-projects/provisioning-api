## Build and Run

### Build for MacOS / Linux / Windows
```bash
GOOS=darwin|linux
GOARCH=amd64

go build ./cmd/main.go
```

### Run
```bash
# Run the output binary
./main

# Run with go run
go run cmd/main.go
```

## Deploying

### Minikube
```bash
# Start minikube
minikube start --vm=true --memory=16g --cpus=8

# Enable ingress
minikube addons enable ingress
```

### Apply the Files
```bash
kubectl create -f deploy/
```
 