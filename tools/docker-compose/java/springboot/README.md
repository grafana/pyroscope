# Spring Boot Demo

Run with:

```bash
./gradlew bootRun
```

Get a CPU profile with:

```bash
go tool pprof -http :6060 "http://localhost:8080/debug/pprof/profile?seconds=10"
```
