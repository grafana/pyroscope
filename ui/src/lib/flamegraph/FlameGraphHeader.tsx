import { css, cx } from '@emotion/css';
import { useEffect, useId, useState } from 'react';
import * as React from 'react';
import { useDebounce, usePrevious } from 'react-use';

import { Icon, type IconType } from '@components/core/Icon';

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
  const [localSearch, setLocalSearch] = useSearchInput(props.search, props.setSearch);

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

  const tableOnly = selectedView === SelectedView.TopTable;
  const viewOptions = getViewOptions(containerWidth, vertical);

  return (
    <div className={cx(styles.header, { [styles.stickyHeader]: stickyHeader })}>
      <div className={styles.inputContainer}>
        <div className={styles.searchWrapper}>
          <input
            type="text"
            className={styles.searchInput}
            value={localSearch || ''}
            onChange={(e) => setLocalSearch(e.currentTarget.value)}
            placeholder="Search..."
          />
          {localSearch !== '' ? (
            <button
              type="button"
              className={styles.clearButton}
              onClick={() => {
                props.setSearch('');
                setLocalSearch('');
              }}
              aria-label="Clear"
            >
              <Icon name="times" size={12} />
              <span>Clear</span>
            </button>
          ) : null}
        </div>
      </div>

      <div className={styles.rightContainer}>
        {showResetButton && (
          <IconBtn
            icon="history-alt"
            label="Reset focus and sandwich state"
            onClick={onReset}
          />
        )}
        <ColorSchemeButton value={colorScheme} onChange={onColorSchemeChange} />
        <div className={cx(styles.btnGroup, styles.spacingRight)}>
          <IconBtn
            icon="angle-double-down"
            label="Expand all groups"
            disabled={tableOnly}
            onClick={() => setCollapsedMap(collapsedMap.setAllCollapsedStatus(false))}
            grouped
          />
          <IconBtn
            icon="angle-double-up"
            label="Collapse all groups"
            disabled={tableOnly}
            onClick={() => setCollapsedMap(collapsedMap.setAllCollapsedStatus(true))}
            grouped
          />
        </div>
        <RadioGroup
          name="text-align"
          value={textAlign}
          onChange={onTextAlignChange}
          disabled={tableOnly}
          className={styles.spacingRight}
          options={[
            { value: 'left', title: 'Align text left', icon: 'align-left' },
            { value: 'right', title: 'Align text right', icon: 'align-right' },
          ]}
        />
        <RadioGroup
          name="selected-view"
          value={selectedView}
          onChange={setSelectedView}
          options={viewOptions.map((o) => ({ value: o.value, title: o.label, label: o.label }))}
        />
        {extraHeaderElements && <div className={styles.extraElements}>{extraHeaderElements}</div>}
      </div>
    </div>
  );
};

function IconBtn({
  icon,
  label,
  onClick,
  disabled,
  grouped,
}: {
  icon: IconType;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  grouped?: boolean;
}) {
  return (
    <button
      type="button"
      className={cx(styles.iconBtn, grouped && styles.iconBtnGrouped, !grouped && styles.spacingRight)}
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
    >
      <Icon name={icon} size={14} />
    </button>
  );
}

type RadioOption<T extends string> = {
  value: T;
  title: string;
  label?: string;
  icon?: IconType;
};

function RadioGroup<T extends string>({
  name,
  value,
  onChange,
  options,
  disabled,
  className,
}: {
  name: string;
  value: T;
  onChange: (v: T) => void;
  options: RadioOption<T>[];
  disabled?: boolean;
  className?: string;
}) {
  const groupId = useId();
  return (
    <div className={cx(styles.radioGroup, className)} role="radiogroup">
      {options.map((opt) => {
        const id = `${groupId}-${opt.value}`;
        const checked = value === opt.value;
        return (
          <React.Fragment key={opt.value}>
            <input
              type="radio"
              id={id}
              name={`${name}-${groupId}`}
              checked={checked}
              disabled={disabled}
              onChange={() => onChange(opt.value)}
              className={styles.radioInput}
              aria-label={opt.title}
            />
            <label htmlFor={id} title={opt.title} className={styles.radioLabel} data-checked={checked}>
              {opt.icon && <Icon name={opt.icon} size={14} />}
              {opt.label && <span>{opt.label}</span>}
            </label>
          </React.Fragment>
        );
      })}
    </div>
  );
}

function getViewOptions(width: number, vertical?: boolean): Array<{ value: SelectedView; label: string }> {
  const options: Array<{ value: SelectedView; label: string }> = [
    { value: SelectedView.TopTable, label: 'Top Table' },
    { value: SelectedView.FlameGraph, label: 'Flame Graph' },
  ];
  if (width >= MIN_WIDTH_TO_SHOW_BOTH_TOPTABLE_AND_FLAMEGRAPH || vertical) {
    options.push({ value: SelectedView.Both, label: 'Both' });
  }
  return options;
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

const styles = {
  header: css({
    label: 'header',
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'space-between',
    width: '100%',
    top: 0,
    gap: 8,
    marginTop: 8,
  }),
  stickyHeader: css({
    zIndex: 1000,
    position: 'sticky',
    background: 'var(--bg-primary)',
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
  spacingRight: css({
    label: 'spacingRight',
    marginRight: 8,
  }),
  extraElements: css({
    label: 'extraElements',
    marginLeft: 8,
  }),
  searchWrapper: css({
    label: 'searchWrapper',
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
  }),
  searchInput: css({
    label: 'searchInput',
    flex: 1,
    height: 28,
    paddingLeft: 8,
    paddingRight: 8,
    background: 'var(--bg-secondary)',
    color: 'var(--text-primary)',
    border: '1px solid var(--border-medium)',
    borderRadius: 'var(--radius-md)',
    fontSize: 'var(--text-sm)',
    outline: 'none',
    '&:focus': {
      borderColor: 'var(--color-primary-border)',
    },
    '&::placeholder': {
      color: 'var(--text-secondary)',
    },
  }),
  clearButton: css({
    label: 'clearButton',
    position: 'absolute',
    right: 4,
    top: 2,
    height: 24,
    display: 'inline-flex',
    alignItems: 'center',
    gap: 4,
    background: 'transparent',
    color: 'var(--text-secondary)',
    border: 'none',
    padding: '0 6px',
    cursor: 'pointer',
    fontSize: 'var(--text-sm)',
    '&:hover': { color: 'var(--text-primary)' },
  }),
  iconBtn: css({
    label: 'iconBtn',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    height: 28,
    minWidth: 28,
    padding: '0 6px',
    background: 'transparent',
    color: 'var(--text-primary)',
    border: '1px solid var(--color-secondary-border)',
    borderRadius: 'var(--radius-md)',
    cursor: 'pointer',
    '&:hover': { background: 'var(--action-hover)' },
    '&:disabled': {
      cursor: 'not-allowed',
      opacity: 0.4,
    },
  }),
  iconBtnGrouped: css({
    label: 'iconBtnGrouped',
    borderRadius: 0,
    marginLeft: -1,
    '&:first-of-type': {
      borderTopLeftRadius: 'var(--radius-md)',
      borderBottomLeftRadius: 'var(--radius-md)',
      marginLeft: 0,
    },
    '&:last-of-type': {
      borderTopRightRadius: 'var(--radius-md)',
      borderBottomRightRadius: 'var(--radius-md)',
    },
  }),
  btnGroup: css({
    label: 'btnGroup',
    display: 'inline-flex',
  }),
  radioGroup: css({
    label: 'radioGroup',
    display: 'inline-flex',
    alignItems: 'center',
  }),
  radioInput: css({
    label: 'radioInput',
    position: 'absolute',
    width: 1,
    height: 1,
    padding: 0,
    margin: -1,
    overflow: 'hidden',
    clip: 'rect(0,0,0,0)',
    whiteSpace: 'nowrap',
    border: 0,
  }),
  radioLabel: css({
    label: 'radioLabel',
    display: 'inline-flex',
    alignItems: 'center',
    gap: 4,
    height: 28,
    padding: '0 10px',
    background: 'transparent',
    color: 'var(--text-secondary)',
    border: '1px solid var(--color-secondary-border)',
    borderRight: 'none',
    fontSize: 'var(--text-sm)',
    cursor: 'pointer',
    '&:first-of-type': {
      borderTopLeftRadius: 'var(--radius-md)',
      borderBottomLeftRadius: 'var(--radius-md)',
    },
    '&:last-of-type': {
      borderRight: '1px solid var(--color-secondary-border)',
      borderTopRightRadius: 'var(--radius-md)',
      borderBottomRightRadius: 'var(--radius-md)',
    },
    '&:hover': { background: 'var(--action-hover)' },
    "&[data-checked='true']": {
      background: 'var(--action-selected)',
      color: 'var(--color-primary-text)',
      borderColor: 'var(--color-primary-border)',
    },
    "input:disabled + &": {
      cursor: 'not-allowed',
      opacity: 0.4,
    },
    "input:focus-visible + &": {
      outline: '2px solid var(--action-focus)',
      outlineOffset: 1,
    },
  }),
};

export default FlameGraphHeader;
