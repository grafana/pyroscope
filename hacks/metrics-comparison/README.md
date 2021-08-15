# Metrics Comparison

Run 2 pyroscope instances, 2 golang agents, a prometheus instance and grafana.
The objective is to be able to easily compare metrics before/after changes.

```
docker-compose up -d
```

to rebuild from local env
```
docker-compose up --remove-orphans -d --force-recreate --build pyroscope_dev
```
