/* eslint-disable import/prefer-default-export */
import React, { ChangeEvent, PureComponent } from 'react';
import { LegacyForms } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { MyDataSourceOptions } from './types';

const { /* SecretFormField, */ FormField } = LegacyForms;

type Props = DataSourcePluginOptionsEditorProps<MyDataSourceOptions>;

// interface State {}

export class ConfigEditor extends PureComponent<Props, unknown> {
  onPathChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      path: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  render() {
    const { options } = this.props;
    const { jsonData } = options;

    return (
      <div className="gf-form-group">
        <div className="gf-form">
          <FormField
            label="Pyroscope instance"
            labelWidth={6}
            inputWidth={20}
            onChange={this.onPathChange}
            value={jsonData.path || ''}
            placeholder="url to your pyroscope instance"
          />
        </div>
      </div>
    );
  }
}
