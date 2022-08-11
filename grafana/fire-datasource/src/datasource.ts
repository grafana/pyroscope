import { DataSourceInstanceSettings } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import {MyDataSourceOptions, Query, ProfileTypeMessage} from './types';

export class DataSource extends DataSourceWithBackend<Query, MyDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<MyDataSourceOptions>) {
    super(instanceSettings);
  }

  async getProfileTypes(): Promise<ProfileTypeMessage[]> {
    return await super.getResource("profileTypes");
  }
}
