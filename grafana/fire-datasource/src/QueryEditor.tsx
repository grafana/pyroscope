import { defaults } from 'lodash';
import React, { PureComponent, FormEvent } from 'react';
import { Input, ButtonCascader, CascaderOption } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from './datasource';
import { defaultQuery, MyDataSourceOptions, ProfileType, Query } from './types';


type Props = QueryEditorProps<DataSource, Query, MyDataSourceOptions>

interface State {
  profileTypes: CascaderOption[]
}

export class QueryEditor extends PureComponent<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = {
      profileTypes: [],
    };
  }

  onProfileTypeChange = (value: string[], selectedOptions: CascaderOption[]) => {
    if (selectedOptions.length === 0) {
      return
    }
    let type = selectedOptions[selectedOptions.length - 1].value as ProfileType;
    this.props.onChange({ ...this.props.query, ProfileType: type });
  };
  onLabelSelectorChange = (value: FormEvent<HTMLInputElement>) => {
    this.props.onChange({ ...this.props.query, LabelSelector: value.currentTarget.value });
  };
  componentDidMount() {
    this.props.datasource.getProfileTypes().then(profileTypes => {
      let mainTypes = new Map<string, CascaderOption>();

      // Classify profile types by name then sample type.
      for (let profileType of profileTypes) {
        if (!mainTypes.has(profileType.name)) {
          mainTypes.set(profileType.name, {
            label: profileType.name,
            value: profileType,
            children: [],
          });
        }
        mainTypes.get(profileType.name)?.children?.push({
          label: profileType.sampleType,
          value: profileType,
        })
      }
      let types = Array.from(mainTypes.values());
      this.setState({
        profileTypes: types,
      });
    });
  };

  render() {
    let query = defaults(this.props.query, defaultQuery);
    return (
      <div className="gf-form">
        <ButtonCascader
          onChange={this.onProfileTypeChange}
          options={this.state.profileTypes}
          icon='process'
        >{query.ProfileType?.Label() || 'Select a profile type'}</ButtonCascader>
        <Input onChange={this.onLabelSelectorChange} value={query.LabelSelector} />
      </div>
    );
  }
}
