import { css } from '@emotion/css';
import { languageDefinition } from './fireql';
import { defaults } from 'lodash';
import React, { useState } from 'react';
import { ButtonCascader, CascaderOption, CodeEditor, Monaco, useStyles2 } from '@grafana/ui';
import { GrafanaTheme2, QueryEditorProps } from '@grafana/data';
import { DataSource } from './datasource';
import { defaultQuery, MyDataSourceOptions, ProfileType, Query } from './types';
import { useMount } from 'react-use';
import type { languages } from 'monaco-editor';

type Props = QueryEditorProps<DataSource, Query, MyDataSourceOptions>;

export function QueryEditor(props: Props) {
  const [profileTypes, setProfileTypes] = useState<CascaderOption[]>([]);

  function onProfileTypeChange(value: string[], selectedOptions: CascaderOption[]) {
    if (selectedOptions.length === 0) {
      return;
    }
    let type = selectedOptions[selectedOptions.length - 1].value as ProfileType;
    props.onChange({ ...props.query, ProfileType: type });
  }

  function onLabelSelectorChange(value: string) {
    props.onChange({ ...props.query, LabelSelector: value });
  }

  useMount(async () => {
    const profileTypes = await props.datasource.getProfileTypes();
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
      });
    }
    let types = Array.from(mainTypes.values());
    setProfileTypes(types);
  });

  let query = defaults(props.query, defaultQuery);
  const selectedProfileName = props.query.ProfileType
    ? props.query.ProfileType.name + ' - ' + props.query.ProfileType.sampleType
    : 'Select a profile type';

  const styles = useStyles2(getStyles);
  return (
    <div className="gf-form">
      <ButtonCascader onChange={onProfileTypeChange} options={profileTypes} icon="process">
        {selectedProfileName}
      </ButtonCascader>
      {/*<Input onChange={this.onLabelSelectorChange} value={query.LabelSelector} />*/}
      <CodeEditor
        value={query.LabelSelector}
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
