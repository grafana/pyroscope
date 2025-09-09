# Migration v1 to v2

## Single binary

### First deploy v1

- Note: Needs persistence enabled otherwise data will be lost after restart

```
helm upgrade \
  pyroscope \
  ./operations/pyroscope/helm/pyroscope \
  --install \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=false \
  --set pyroscope.persistence.enabled=true
```

### Step: 2: Now deploy v2 and enable dual ingest

- Note: The python command will switch to the v2 write path 10 minutes after it is run.

```
helm upgrade \
  pyroscope \
  ./operations/pyroscope/helm/pyroscope \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true \
  --set pyroscope.persistence.enabled=true \
  --set architecture.storage.migration.queryBackendFrom=$(python3 -c "import datetime; print((datetime.datetime.now(datetime.UTC)+ datetime.timedelta(minutes = 10)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
```

### Step 3: Now remove v1 components, this will loose all data before Step 2

```
helm upgrade \
  pyroscope \
  ./operations/pyroscope/helm/pyroscope \
  --set architecture.storage.v1=false \
  --set architecture.storage.v2=true \
  --set pyroscope.persistence.enabled=true
```


## Micro-Services

### First deploy v1

- Note: Needs persistence enabled otherwise data will be lost after restart

```
helm upgrade \
  pyroscope \
  ./operations/pyroscope/helm/pyroscope \
  --install \
  --set architecture.microservices.enabled=true \
  --set minio.enabled=true \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=false \
  --set pyroscope.persistence.enabled=true
```

### Step: 2: Now deploy v2 and enable dual ingest

- Note: The python command will switch to the v2 write path 10 minutes after it is run.

```
helm upgrade \
  pyroscope \
  ./operations/pyroscope/helm/pyroscope \
  --set architecture.microservices.enabled=true \
  --set minio.enabled=true \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true \
  --set pyroscope.persistence.enabled=true \
  --set architecture.storage.migration.queryBackendFrom=$(python3 -c "import datetime; print((datetime.datetime.now(datetime.UTC)+ datetime.timedelta(minutes = 10)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
```


### Step 3: Now remove v1 components, this will loose all data before Step 2

```
helm upgrade \
  pyroscope \
  ./operations/pyroscope/helm/pyroscope \
  --set architecture.microservices.enabled=true \
  --set minio.enabled=true \
  --set architecture.storage.v1=false \
  --set architecture.storage.v2=true \
  --set pyroscope.persistence.enabled=true

