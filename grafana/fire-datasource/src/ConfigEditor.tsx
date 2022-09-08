import React from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { MyDataSourceOptions } from './types';
import { DataSourceHttpSettings } from '@grafana/ui';

interface Props extends DataSourcePluginOptionsEditorProps<MyDataSourceOptions> {}

export const ConfigEditor = (props: Props) => {
  const { options, onOptionsChange } = props;

  return (
    <>
      <DataSourceHttpSettings
        defaultUrl={'http://localhost:4100'}
        dataSourceConfig={options}
        showAccessOptions={false}
        onChange={onOptionsChange}
      />
    </>
  );
};
