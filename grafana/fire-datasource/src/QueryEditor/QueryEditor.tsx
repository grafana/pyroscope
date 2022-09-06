import { defaults } from 'lodash';
import React, { useMemo, useState } from 'react';
import { useMount } from 'react-use';

import { ButtonCascader, CascaderOption } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';

import { FireDataSource } from '../datasource';
import { defaultQuery, MyDataSourceOptions, ProfileTypeMessage, Query } from '../types';
import { LabelsEditor } from './LabelsEditor';

type Props = QueryEditorProps<FireDataSource, Query, MyDataSourceOptions>;

export function QueryEditor(props: Props) {
  const [profileTypes, setProfileTypes] = useState<ProfileTypeMessage[]>([]);

  function onProfileTypeChange(value: string[], selectedOptions: CascaderOption[]) {
    if (selectedOptions.length === 0) {
      return;
    }
    const id = selectedOptions[selectedOptions.length - 1].value as string;
    props.onChange({ ...props.query, profileTypeId: id });
  }

  function onLabelSelectorChange(value: string) {
    props.onChange({ ...props.query, labelSelector: value });
  }

  useMount(async () => {
    const profileTypes = await props.datasource.getProfileTypes();
    setProfileTypes(profileTypes);
    // todo remove me.
    console.log(await props.datasource.getLabelNames());
  });

  // Turn profileTypes into cascader options
  const cascaderOptions = useMemo(() => {
    let mainTypes = new Map<string, CascaderOption>();
    // Classify profile types by name then sample type.
    for (let profileType of profileTypes) {
      if (!mainTypes.has(profileType.name)) {
        mainTypes.set(profileType.name, {
          label: profileType.name,
          value: profileType.ID,
          children: [],
        });
      }
      mainTypes.get(profileType.name)?.children?.push({
        label: profileType.sample_type,
        value: profileType.ID,
      });
    }
    return Array.from(mainTypes.values());
  }, [profileTypes]);

  const selectedProfileName = useMemo(() => {
    if (!profileTypes) {
      return 'Loading';
    }
    const profile = profileTypes.find((type) => type.ID === props.query.profileTypeId);
    if (!profile) {
      return 'Select a profile type';
    }

    return profile.name + ' - ' + profile.sample_type;
  }, [props.query.profileTypeId, profileTypes]);

  let query = defaults(props.query, defaultQuery);

  return (
    <div className="gf-form">
      <ButtonCascader onChange={onProfileTypeChange} options={cascaderOptions} icon="process">
        {selectedProfileName}
      </ButtonCascader>
      <LabelsEditor value={query.labelSelector} onChange={onLabelSelectorChange} datasource={props.datasource} />
    </div>
  );
}
