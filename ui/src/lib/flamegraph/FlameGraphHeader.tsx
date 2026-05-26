import { useEffect, useId, useState } from 'react';
import * as React from 'react';

import { Icon, type IconType } from '@components/core/Icon';

import { ColorSchemeButton } from './ColorSchemeButton';
import { type CollapsedMap } from './FlameGraph/dataTransform';
import { MIN_WIDTH_TO_SHOW_BOTH_TOPTABLE_AND_FLAMEGRAPH } from './constants';
import { cx } from './cx';
import { useDebounce, usePrevious } from './hooks';
import { type ColorScheme, SelectedView, type TextAlign } from './types';

import './FlameGraphHeader.css';

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
    <div className={cx('fg-header', { 'fg-header-sticky': stickyHeader })}>
      <div className="fg-header-input-container">
        <div className="fg-header-search-wrapper">
          <input
            type="text"
            className="fg-header-search-input"
            value={localSearch || ''}
            onChange={(e) => setLocalSearch(e.currentTarget.value)}
            placeholder="Search..."
          />
          {localSearch !== '' ? (
            <button
              type="button"
              className="fg-header-clear-button"
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

      <div className="fg-header-right">
        {showResetButton && (
          <IconBtn
            icon="history-alt"
            label="Reset focus and sandwich state"
            onClick={onReset}
          />
        )}
        <ColorSchemeButton value={colorScheme} onChange={onColorSchemeChange} />
        <div className={cx('fg-header-btn-group', 'fg-header-spacing-right')}>
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
          className="fg-header-spacing-right"
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
        {extraHeaderElements && <div className="fg-header-extra-elements">{extraHeaderElements}</div>}
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
      className={cx('fg-header-icon-btn', grouped && 'fg-header-icon-btn-grouped', !grouped && 'fg-header-spacing-right')}
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
    <div className={cx('fg-header-radio-group', className)} role="radiogroup">
      {options.map((opt) => {
        const id = `${groupId}-${opt.value}`;
        const checked = value === opt.value;
        return (
          <span key={opt.value} className="fg-header-radio-cell">
            {/* Input must remain a preceding sibling of the label — e2e
                tests use `label/preceding-sibling::input` XPath to find it. */}
            <input
              type="radio"
              id={id}
              name={`${name}-${groupId}`}
              checked={checked}
              disabled={disabled}
              onChange={() => onChange(opt.value)}
              className="fg-header-radio-input"
              aria-label={opt.title}
            />
            <label htmlFor={id} title={opt.title} className="fg-header-radio-label" data-checked={checked}>
              {opt.icon && <Icon name={opt.icon} size={14} />}
              {opt.label && <span>{opt.label}</span>}
            </label>
          </span>
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


export default FlameGraphHeader;
