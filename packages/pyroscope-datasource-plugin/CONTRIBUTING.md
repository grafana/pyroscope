# Pyroscope Grafana Panel Plugin

**Important: Grafana version 7.2 or later required**

# Developing

1. to build the app:
`yarn dev:datasource`

2. open grafana:
`docker-compose up`

3. open a dashboard

4. every time you change code the app will be rebuilt, and you will have to refresh the dashboard page


# Testing
## E2E
From the root of this repository, run either
* `cy:datasource:open` -> to develop locally
* `cy:datasource:ci` -> to run in ci (it will use headless mode)
* `cy:datasource:ss` -> to take screenshots, it will start a container using `docker`
* `cy:datasource:ss-check` -> to verify the screenshots match, it will start a container using `docker`

All these commands assume:
* an anonymous grafana instance (as in there's no login required)
* running on http://localhost:3000
* a dashboard with UID `single-panel`

