/* eslint-disable import/prefer-default-export */
import { PanelPlugin } from '@grafana/data';
import { loadPluginCss } from '@grafana/runtime';
import { SimpleOptions } from './types';
import { SimplePanel } from './SimplePanel';
import '@pyroscope/flamegraph/dist/index.css';
import './styles.css';

// Since our webpack config generates a single css file
// We have to load it somehow
// This could be solved differently, by using style-loader and injecting the css in the DOM using javascript
loadPluginCss({
  light: 'plugins/pyroscope-panel/module.css',
  dark: 'plugins/pyroscope-panel/module.css',
}).catch(() => {
  console.error('Failed to load CSS.');
});

export const plugin = new PanelPlugin<SimpleOptions>(
  SimplePanel
).setPanelOptions((builder) => {
  return builder
    .addBooleanSwitch({
      description:
        'Whether to show the toolbar. Keep in mind most of the same functionality can be accessed by right-clicking the flamegraph.',
      path: 'showToolbar',
      name: 'Show toolbar',
      defaultValue: false,
    })
    .addSelect({
      path: 'displayOnly',
      name: 'Display only',
      description: 'Only display a single view, not allowing to change',
      defaultValue: 'flamegraph',
      settings: {
        options: [
          { value: 'flamegraph', label: 'Flamegraph' },
          { value: 'table', label: 'Table' },
          { value: 'both', label: 'Flamegraph + Table' },
          { value: 'sandwich', label: 'Sandwich' },
          { value: 'off', label: 'Off' },
        ],
      },
    });
});
