local config = import 'config.libsonnet';
local dashboard = import './lib/dashboard.libsonnet';

(config + dashboard).dashboard
