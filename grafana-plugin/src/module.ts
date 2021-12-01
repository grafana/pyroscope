import { PanelPlugin } from '@grafana/data';
import { SimpleOptions } from './types';
import { SimplePanel } from './SimplePanel';
import { loadPluginCss } from '@grafana/runtime';
import '../../webapp/sass/profile.scss';

// We don't support light mode yet
loadPluginCss({
  dark: 'plugins/pyroscope-panel/module.css',
  light: 'plugins/pyroscope-panel/module.css',
});
export const plugin = new PanelPlugin<SimpleOptions>(
  SimplePanel
).setPanelOptions((builder) => {
  return builder
    .addTextInput({
      path: 'text',
      name: 'Simple text option',
      description: 'Description of panel option',
      defaultValue: 'Default value of text input option',
    })
    .addBooleanSwitch({
      path: 'showSeriesCount',
      name: 'Show series counter',
      defaultValue: false,
    })
    .addRadio({
      path: 'seriesCountSize',
      defaultValue: 'sm',
      name: 'Series counter size',
      settings: {
        options: [
          {
            value: 'sm',
            label: 'Small',
          },
          {
            value: 'md',
            label: 'Medium',
          },
          {
            value: 'lg',
            label: 'Large',
          },
        ],
      },
      showIf: (config) => config.showSeriesCount,
    });
});
