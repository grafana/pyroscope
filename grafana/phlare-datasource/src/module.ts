import { DataSourcePlugin } from '@grafana/data';
import { PhlareDataSource } from './datasource';
import { ConfigEditor } from './ConfigEditor';
import { QueryEditor } from './QueryEditor/QueryEditor';
import { Query, PhlareDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<PhlareDataSource, Query, PhlareDataSourceOptions>(PhlareDataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
