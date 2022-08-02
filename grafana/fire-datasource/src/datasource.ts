import { DataSourceInstanceSettings } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import { MyDataSourceOptions, Query, ProfileType } from './types';

export class DataSource extends DataSourceWithBackend<Query, MyDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<MyDataSourceOptions>) {
    super(instanceSettings);
  }

  async getProfileTypes(): Promise<ProfileType[]> {
    const profiles = await super.getResource("profileTypes");
    return profiles.map((profileType: any) => {
      return new ProfileType(profileType.ID, profileType.name, profileType.periodType, profileType.periodUnit, profileType.sampleType, profileType.sampleUnit);
    });
  }
}
