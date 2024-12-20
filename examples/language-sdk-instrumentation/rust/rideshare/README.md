## Rust Example

To run the rideshare example run the following commands:
```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker compose up --build

# Reset the database (if needed):
# docker compose down
```

Example output:
![Image](https://github.com/user-attachments/assets/0c402f72-3936-4c27-a22e-9b7af456fb21)
![Image](https://github.com/user-attachments/assets/b5f51af8-57d6-4dd6-b98e-44f5162d2ca2)


