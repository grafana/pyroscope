/* eslint-disable import/prefer-default-export */
import { PanelPlugin } from '@grafana/data';
import { loadPluginCss } from '@grafana/runtime';
import { SimpleOptions } from './types';
import { SimplePanel } from './SimplePanel';
import '../../../webapp/sass/profile.scss';
import './styles.css';

// We don't support light mode yet
loadPluginCss({
  dark: 'plugins/pyroscope-panel/module.css',
  light: 'plugins/pyroscope-panel/module.css',
});

export const plugin = new PanelPlugin<SimpleOptions>(
  SimplePanel
).setPanelOptions((builder) => {
  return builder.addBooleanSwitch({
    description:
      'Whether to show the toolbar. Keep in mind most of the same functionality can be accessed by right-clicking the flamegraph.',
    path: 'showToolbar',
    name: 'Show toolbar',
    defaultValue: false,
  });
});
