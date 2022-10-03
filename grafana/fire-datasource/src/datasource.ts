import { DataSourceInstanceSettings } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import { FireDataSourceOptions, Query, ProfileTypeMessage, SeriesMessage } from './types';

export class FireDataSource extends DataSourceWithBackend<Query, FireDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<FireDataSourceOptions>) {
    super(instanceSettings);
  }

  async getProfileTypes(): Promise<ProfileTypeMessage[]> {
    return await super.getResource('profileTypes');
  }

  async getSeries(): Promise<SeriesMessage> {
    // For now, we send empty matcher to get all the series
    return await super.getResource('series', { matchers: ['{}'] });
  }

  async getLabelNames(): Promise<string[]> {
    return await super.getResource("labelNames");
  }
}
