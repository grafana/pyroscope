import { css, cx } from '@emotion/css';
import { useEffect, useState } from 'react';
import * as React from 'react';
import { useDebounce, usePrevious } from 'react-use';

import { type GrafanaTheme2, type SelectableValue } from '@grafana/data';
import { Button, ButtonGroup, Input, RadioButtonGroup, useStyles2 } from '@grafana/ui';

import { ColorSchemeButton } from './ColorSchemeButton';
import { type CollapsedMap } from './FlameGraph/dataTransform';
import { MIN_WIDTH_TO_SHOW_BOTH_TOPTABLE_AND_FLAMEGRAPH } from './constants';
import { type ColorScheme, SelectedView, type TextAlign } from './types';

type Props = {
  search: string;
  setSearch: (search: string) => void;
  selectedView: SelectedView;
  setSelectedView: (view: SelectedView) => void;
  containerWidth: number;
  onReset: () => void;
  textAlign: TextAlign;
  onTextAlignChange: (align: TextAlign) => void;
  showResetButton: boolean;
  colorScheme: ColorScheme;
  onColorSchemeChange: (colorScheme: ColorScheme) => void;
  stickyHeader: boolean;
  vertical?: boolean;
  setCollapsedMap: (collapsedMap: CollapsedMap) => void;
  collapsedMap: CollapsedMap;

  extraHeaderElements?: React.ReactNode;
};

const FlameGraphHeader = (props: Props) => {
  const styles = useStyles2(getStyles);
  const [localSearch, setLocalSearch] = useSearchInput(props.search, props.setSearch);

  const suffix =
    localSearch !== '' ? (
      <Button
        icon="times"
        fill="text"
        size="sm"
        onClick={() => {
          // We could set only one and wait them to sync but there is no need to debounce this.
          props.setSearch('');
          setLocalSearch('');
        }}
      >
        Clear
      </Button>
    ) : null;

  const {
    selectedView,
    setSelectedView,
    containerWidth,
    onReset,
    textAlign,
    onTextAlignChange,
    showResetButton,
    colorScheme,
    onColorSchemeChange,
    stickyHeader,
    extraHeaderElements,
    vertical,
    setCollapsedMap,
    collapsedMap,
  } = props;

  return (
    <div className={cx(styles.header, { [styles.stickyHeader]: stickyHeader })}>
      <div className={styles.inputContainer}>
        <Input
          value={localSearch || ''}
          onChange={(v) => {
            setLocalSearch(v.currentTarget.value);
          }}
          placeholder={'Search...'}
          suffix={suffix}
        />
      </div>

      <div className={styles.rightContainer}>
        {showResetButton && (
          <Button
            variant={'secondary'}
            fill={'outline'}
            size={'sm'}
            icon={'history-alt'}
            tooltip={'Reset focus and sandwich state'}
            onClick={() => {
              onReset();
            }}
            className={styles.buttonSpacing}
            aria-label={'Reset focus and sandwich state'}
          />
        )}
        <ColorSchemeButton value={colorScheme} onChange={onColorSchemeChange} />
        <ButtonGroup className={styles.buttonSpacing}>
          <Button
            variant={'secondary'}
            fill={'outline'}
            size={'sm'}
            tooltip={'Expand all groups'}
            onClick={() => {
              setCollapsedMap(collapsedMap.setAllCollapsedStatus(false));
            }}
            aria-label={'Expand all groups'}
            icon={'angle-double-down'}
            disabled={selectedView === SelectedView.TopTable}
          />
          <Button
            variant={'secondary'}
            fill={'outline'}
            size={'sm'}
            tooltip={'Collapse all groups'}
            onClick={() => {
              setCollapsedMap(collapsedMap.setAllCollapsedStatus(true));
            }}
            aria-label={'Collapse all groups'}
            icon={'angle-double-up'}
            disabled={selectedView === SelectedView.TopTable}
          />
        </ButtonGroup>
        <RadioButtonGroup<TextAlign>
          size="sm"
          disabled={selectedView === SelectedView.TopTable}
          options={alignOptions}
          value={textAlign}
          onChange={onTextAlignChange}
          className={styles.buttonSpacing}
        />
        <RadioButtonGroup<SelectedView>
          size="sm"
          options={getViewOptions(containerWidth, vertical)}
          value={selectedView}
          onChange={setSelectedView}
        />
        {extraHeaderElements && <div className={styles.extraElements}>{extraHeaderElements}</div>}
      </div>
    </div>
  );
};

export const alignOptions: Array<SelectableValue<TextAlign>> = [
  { value: 'left', description: 'Align text left', icon: 'align-left' },
  { value: 'right', description: 'Align text right', icon: 'align-right' },
];

function getViewOptions(width: number, vertical?: boolean): Array<SelectableValue<SelectedView>> {
  let viewOptions: Array<{ value: SelectedView; label: string; description: string }> = [
    { value: SelectedView.TopTable, label: 'Top Table', description: 'Only show top table' },
    { value: SelectedView.FlameGraph, label: 'Flame Graph', description: 'Only show flame graph' },
  ];

  if (width >= MIN_WIDTH_TO_SHOW_BOTH_TOPTABLE_AND_FLAMEGRAPH || vertical) {
    viewOptions.push({
      value: SelectedView.Both,
      label: 'Both',
      description: 'Show both the top table and flame graph',
    });
  }

  return viewOptions;
}

function useSearchInput(
  search: string,
  setSearch: (search: string) => void
): [string | undefined, (search: string) => void] {
  const [localSearchState, setLocalSearchState] = useState(search);
  const prevSearch = usePrevious(search);

  // Debouncing cause changing parent search triggers rerender on both the flamegraph and table
  useDebounce(
    () => {
      setSearch(localSearchState);
    },
    250,
    [localSearchState]
  );

  // Make sure we still handle updates from parent (from clicking on a table item for example). We check if the parent
  // search value changed to something that isn't our local value.
  useEffect(() => {
    if (prevSearch !== search && search !== localSearchState) {
      setLocalSearchState(search);
    }
  }, [search, prevSearch, localSearchState]);

  return [localSearchState, setLocalSearchState];
}

const getStyles = (theme: GrafanaTheme2) => ({
  header: css({
    label: 'header',
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'space-between',
    width: '100%',
    top: 0,
    gap: theme.spacing(1),
    marginTop: theme.spacing(1),
  }),
  stickyHeader: css({
    zIndex: theme.zIndex.navbarFixed,
    position: 'sticky',
    background: theme.colors.background.primary,
  }),
  inputContainer: css({
    label: 'inputContainer',
    flexGrow: 1,
    minWidth: '150px',
    maxWidth: '350px',
  }),
  rightContainer: css({
    label: 'rightContainer',
    display: 'flex',
    alignItems: 'flex-start',
    flexWrap: 'wrap',
  }),
  buttonSpacing: css({
    label: 'buttonSpacing',
    marginRight: theme.spacing(1),
  }),
  resetButton: css({
    label: 'resetButton',
    display: 'flex',
    marginRight: theme.spacing(2),
  }),
  resetButtonIconWrapper: css({
    label: 'resetButtonIcon',
    padding: '0 5px',
    color: theme.colors.text.disabled,
  }),
  extraElements: css({
    label: 'extraElements',
    marginLeft: theme.spacing(1),
  }),
});

export default FlameGraphHeader;
