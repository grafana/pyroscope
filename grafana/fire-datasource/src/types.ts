import { DataQuery, DataSourceJsonData } from '@grafana/data';

export interface Query extends DataQuery {
  LabelSelector: string;
  ProfileType?: ProfileType;
}

export class ProfileType {

  constructor(public ID: string,
    public name: string,
    public periodType: string,
    public periodUnit: string,
    public sampleType: string,
    public sampleUnit: string) {
    this.ID = ID;
    this.name = name;
    this.periodType = periodType;
    this.periodUnit = periodUnit;
    this.sampleType = sampleType;
    this.sampleUnit = sampleUnit;
  }
  Label(): string {
    return this.name + " - " + this.sampleType;
  }
}

export const defaultQuery: Partial<Query> = {
  LabelSelector: "{}",
}

/**
 * These are options configured for each DataSource instance.
 */
export interface MyDataSourceOptions extends DataSourceJsonData {
}
