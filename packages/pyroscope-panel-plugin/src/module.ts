/* eslint-disable import/prefer-default-export */
import { PanelPlugin } from '@grafana/data';
import { SimpleOptions } from './types';
import { SimplePanel } from './SimplePanel';
import '@pyroscope/flamegraph/dist/index.css';
import './styles.css';

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
