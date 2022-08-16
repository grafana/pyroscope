import React, { useEffect, useRef } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { CodeEditor, Monaco, useStyles2 } from '@grafana/ui';
import type { languages } from 'monaco-editor';
import { useAsync } from 'react-use';

import { languageDefinition } from '../fireql';
import { FireDataSource } from '../datasource';
import {CompletionProvider} from './autocompletition';

interface Props {
  value: string;
  onChange: (val: string) => void;
  datasource: FireDataSource;
}

export function LabelsEditor(props: Props) {
  const providerRef = useRef<CompletionProvider>(new CompletionProvider())

  const seriesResult = useAsync(() => {
    return props.datasource.getSeries();
  });

  if (seriesResult.value) {
    providerRef.current.setSeries(seriesResult.value)
  }

  const autocompleteDisposeFun = useRef<(() => void) | null>(null);
  useEffect(() => {
    // when we unmount, we unregister the autocomplete-function, if it was registered
    return () => {
      autocompleteDisposeFun.current?.();
    };
  }, []);


  const styles = useStyles2(getStyles);
  return (
    <CodeEditor
      value={props.value}
      language={langId}
      onBlur={props.onChange}
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
      onEditorDidMount={(editor, monaco) => {
        providerRef.current.editor = editor
        providerRef.current.monaco = monaco

        const { dispose } = monaco.languages.registerCompletionItemProvider(langId, providerRef.current);
        autocompleteDisposeFun.current = dispose;
      }}
    />
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
