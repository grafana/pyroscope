import { css } from '@emotion/css';
import { defaults } from 'lodash';
import React, { useState, useMemo } from 'react';
import type { languages } from 'monaco-editor';
import { useMount } from 'react-use';

import { ButtonCascader, CascaderOption, CodeEditor, Monaco, useStyles2 } from '@grafana/ui';
import { GrafanaTheme2, QueryEditorProps } from '@grafana/data';

import { languageDefinition } from './fireql';
import { DataSource } from './datasource';
import { defaultQuery, MyDataSourceOptions, ProfileTypeMessage, Query } from './types';

type Props = QueryEditorProps<DataSource, Query, MyDataSourceOptions>;

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

  const styles = useStyles2(getStyles);
  return (
    <div className="gf-form">
      <ButtonCascader onChange={onProfileTypeChange} options={cascaderOptions} icon="process">
        {selectedProfileName}
      </ButtonCascader>
      <CodeEditor
        value={query.labelSelector}
        language={langId}
        onBlur={onLabelSelectorChange}
        height={'30px'}
        containerStyles={styles.queryField}
        monacoOptions={{
          folding: false,
          fontSize: 14,
          lineNumbers: 'off',
          overviewRulerLanes: 0,
          renderLineHighlight: 'none',
          scrollbar: {
            vertical: 'hidden',
            verticalScrollbarSize: 8, // used as "padding-right"
            horizontal: 'hidden',
            horizontalScrollbarSize: 0,
          },
          scrollBeyondLastLine: false,
          wordWrap: 'on',
        }}
        onBeforeEditorMount={ensureFireQL}
      />
    </div>
  );
}

// we must only run the setup code once
let fireqlSetupDone = false;
const langId = 'fireql';

function ensureFireQL(monaco: Monaco) {
  if (fireqlSetupDone === false) {
    fireqlSetupDone = true;
    const { aliases, extensions, mimetypes, def } = languageDefinition;
    monaco.languages.register({ id: langId, aliases, extensions, mimetypes });
    monaco.languages.setMonarchTokensProvider(langId, def.language as languages.IMonarchLanguage);
    monaco.languages.setLanguageConfiguration(langId, def.languageConfiguration as languages.LanguageConfiguration);
  }
}

const getStyles = (theme: GrafanaTheme2) => {
  return {
    queryField: css`
      border-radius: ${theme.shape.borderRadius()};
      border: 1px solid ${theme.components.input.borderColor};
      flex: 1;
    `,
  };
};
