local config = import 'config.libsonnet';
local dashboard = import './lib/dashboard.libsonnet';

(config + dashboard + {
  _config+:: {
    benchmark: true,
  }
}).dashboard
