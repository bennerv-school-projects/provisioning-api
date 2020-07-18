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
minikube start --vm=true --memory=8g --cpus=4 --nodes=2

# Enable ingress
minikube addons enable ingress
```

### Create Pull Secret

```bash
kubectl -n provisioner create secret docker-registry bvesel-pull-secret \
  --docker-username=bennerv \
  --docker-password=801e3784-276d-451e-a3d9-1d714cebaab0 \
  --docker-email=test@gmail.com
```

### Apply the Files
```bash
kubectl create -f deploy/01-namespace.yaml
kubectl create -f deploy/02-priviledges.yaml
kubectl create -f deploy/03-service.yaml
kubectl create -f deploy/04-deployment.yaml
kubectl create -f deploy/05-ingress.yaml
```
 