This directory contains Fire datasource and the FlameGraph panel. 

To be able to test both in Grafana you have to set up your environment in a specific way.

#### Setup Grafana
- Checkout https://github.com/grafana/grafana/pull/52057 in Grafana repo.
- In grafana repo packages/grafana-data/package.json, change `"@grafana/schema": "9.1.0-pre",` to `"@grafana/schema": "9.0.4",`. We will link @grafana/data later on and the `9.1.0-pre` version would not be recognized from Fire repo 
#### Create symbolic links:
Create a links in your Grafana plugin directory so Grafana will lead the plugins on startup.
- `cd $GRAFANA_REPO/data/plugins`
- `ln -s $FIRE_REPO/grafana/flamegraph`
- `ln -s $FIRE_REPO/grafana/fire-datasource`

#### Setup and build plugins:
- `cd $FIRE_REPO/grafana/flamegraph`
- `yarn link grafana/packages/grafana-data` this will change the `resolutions` part in package.json TODO: check if this can be relative path or how to prevent rewriting this all the time.
- `yarn install`
- `yarn build` or `yarn watch`
- `cd $FIRE_REPO/grafana/fire-datasource`
- `yarn install`
- `yarn build` or `yarn watch`
- `mage -v` or `bra run`
- `bra run` will also reload the plugin in your running grafana instance, if you don't use it restart Grafana if it was already running to pick up new plugin state.
