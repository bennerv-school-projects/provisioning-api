### Build

### Create Pull Secret

```bash
kubectl -n provisioner create secret docker-registry bvesel-pull-secret \
  --docker-username=bennerv \
  --docker-password=PASSWORD \
  --docker-email=test@gmail.com
```

 