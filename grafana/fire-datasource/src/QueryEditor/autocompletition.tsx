import type { Monaco, monacoTypes } from '@grafana/ui';
import {SeriesMessage} from "../types";

// export function getCompletionProvider(
//   monaco: Monaco,
//   editor: monacoTypes.editor.IStandaloneCodeEditor
// ): monacoTypes.languages.CompletionItemProvider {
//   const provideCompletionItems = (
//     model: monacoTypes.editor.ITextModel,
//     position: monacoTypes.Position
//   ): monacoTypes.languages.ProviderResult<monacoTypes.languages.CompletionList> => {
//     // if the model-id does not match, then this call is from a different editor-instance,
//     // not "our instance", so return nothing
//     if (editor.getModel()?.id !== model.id) {
//       return { suggestions: [] };
//     }
//
//     const { range, offset } = getRangeAndOffset(monaco, model, position);
//     const situation = getSituation(model.getValue(), offset);
//     const completionsPromise = situation != null ? getCompletions(situation) : Promise.resolve([]);
//
//     return completionsPromise.then((items) => {
//       // monaco by-default alphabetically orders the items.
//       // to stop it, we use a number-as-string sortkey,
//       // so that monaco keeps the order we use
//       const maxIndexDigits = items.length.toString().length;
//       const suggestions: monacoTypes.languages.CompletionItem[] = items.map((item, index) => ({
//         kind: getMonacoCompletionItemKind(item.type, monaco),
//         label: item.label,
//         insertText: item.insertText,
//         detail: item.detail,
//         documentation: item.documentation,
//         sortText: index.toString().padStart(maxIndexDigits, '0'), // to force the order we have
//         range,
//         command: item.triggerOnInsert
//           ? {
//               id: 'editor.action.triggerSuggest',
//               title: '',
//             }
//           : undefined,
//       }));
//       return { suggestions };
//     });
//   };
//
//   return {
//     triggerCharacters: ['{', ',', '[', '(', '=', '~', ' ', '"'],
//     provideCompletionItems,
//   };
// }

export class CompletionProvider implements monacoTypes.languages.CompletionItemProvider {
  triggerCharacters = ['{', ',', '[', '(', '=', '~', ' ', '"'];
  labels: { [label: string]: Set<string> } = {};
  monaco: Monaco | undefined;
  editor: monacoTypes.editor.IStandaloneCodeEditor | undefined;

  provideCompletionItems(
    model: monacoTypes.editor.ITextModel,
    position: monacoTypes.Position
  ): monacoTypes.languages.ProviderResult<monacoTypes.languages.CompletionList> {
    // Should not happen, this should not be called before it is initialized
    if (!(this.monaco && this.editor)) {
      throw new Error('provideCompletionItems called before CompletionProvider was initialized');
    }

    // if the model-id does not match, then this call is from a different editor-instance,
    // not "our instance", so return nothing
    if (this.editor.getModel()?.id !== model.id) {
      return { suggestions: [] };
    }

    const { range, offset } = getRangeAndOffset(this.monaco, model, position);
    const situation = getSituation(model.getValue(), offset);
    const completionItems = situation != null ? this.getCompletions(situation) : [];

    // monaco by-default alphabetically orders the items.
    // to stop it, we use a number-as-string sortkey,
    // so that monaco keeps the order we use
    const maxIndexDigits = completionItems.length.toString().length;
    const suggestions: monacoTypes.languages.CompletionItem[] = completionItems.map((item, index) => ({
      kind: getMonacoCompletionItemKind(item.type, this.monaco!),
      label: item.label,
      insertText: item.insertText,
      detail: item.detail,
      documentation: item.documentation,
      sortText: index.toString().padStart(maxIndexDigits, '0'), // to force the order we have
      range,
      command: item.triggerOnInsert
        ? {
            id: 'editor.action.triggerSuggest',
            title: '',
          }
        : undefined,
    }));
    return { suggestions };
  }

  setSeries(series: SeriesMessage) {
    this.labels = series.reduce((acc, serie) => {
      const seriesLabels = serie.labels.reduce((acc, labelValue) => {
        acc[labelValue.name] = acc[labelValue.name] || new Set()
        acc[labelValue.name].add(labelValue.value)
        return acc
      }, {} as {[label: string]: Set<string>})

      for (const label of Object.keys(seriesLabels)) {
        acc[label] = new Set([...(acc[label] || []), ...seriesLabels[label]])
      }
      return acc
    }, {} as {[label: string]: Set<string>})
  }

  private getCompletions(situation: Situation): Completion[] {
    if (!Object.keys(this.labels).length) {
      return [];
    }
    switch (situation.type) {
      case 'EMPTY': {
        return [];
      }
      case 'IN_LABEL_SELECTOR_NO_LABEL_NAME':
        return Object.keys(this.labels).map(key => {
          return {
            label: key,
            insertText: key,
            type: 'LABEL_NAME'
          }
        });
      case 'IN_LABEL_SELECTOR_WITH_LABEL_NAME':
        return Array.from(this.labels[situation.labelName].values()).map(key => {
          return {
            label: key,
            insertText: key,
            type: 'LABEL_VALUE'
          }
        });
      default:
        throw new Error(`Unexpected situation ${situation}`);
    }
  }
}

function getMonacoCompletionItemKind(type: CompletionType, monaco: Monaco): monacoTypes.languages.CompletionItemKind {
  switch (type) {
    case 'LABEL_NAME':
      return monaco.languages.CompletionItemKind.Enum;
    case 'LABEL_VALUE':
      return monaco.languages.CompletionItemKind.EnumMember;
    default:
      throw new Error(`Unexpected CompletionType: ${type}`);
  }
}

export type CompletionType = 'LABEL_NAME' | 'LABEL_VALUE';
type Completion = {
  type: CompletionType;
  label: string;
  insertText: string;
  detail?: string;
  documentation?: string;
  triggerOnInsert?: boolean;
};

export type Label = {
  name: string;
  value: string;
};

export type Situation =
  | {
      type: 'AT_ROOT';
    }
  | {
      type: 'EMPTY';
    }
  | {
      type: 'IN_LABEL_SELECTOR_NO_LABEL_NAME';
      metricName?: string;
      otherLabels: Label[];
    }
  | {
      type: 'IN_LABEL_SELECTOR_WITH_LABEL_NAME';
      metricName?: string;
      labelName: string;
      betweenQuotes: boolean;
      otherLabels: Label[];
    };

function getSituation(value: string, offest: number): Situation {
  // TODO: parse the value
  return {
    type: 'IN_LABEL_SELECTOR_NO_LABEL_NAME',
    metricName: 'foo',
    otherLabels: [],
  };
}

function getRangeAndOffset(monaco: Monaco, model: monacoTypes.editor.ITextModel, position: monacoTypes.Position) {
  const word = model.getWordAtPosition(position);
  const range =
    word != null
      ? monaco.Range.lift({
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        })
      : monaco.Range.fromPositions(position);

  // documentation says `position` will be "adjusted" in `getOffsetAt`
  // i don't know what that means, to be sure i clone it
  const positionClone = {
    column: position.column,
    lineNumber: position.lineNumber,
  };

  const offset = model.getOffsetAt(positionClone);
  return { offset, range };
}
