import { DataSourcePlugin } from '@grafana/data';
import { FireDataSource } from './datasource';
import { ConfigEditor } from './ConfigEditor';
import { QueryEditor } from './QueryEditor/QueryEditor';
import { Query, MyDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<FireDataSource, Query, MyDataSourceOptions>(FireDataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
