import React, {
  ReactNode,
  RefObject,
  useState,
  useRef,
  useLayoutEffect,
  isValidElement,
  memo,
} from 'react';
import classNames from 'classnames/bind';
import { faUndo } from '@fortawesome/free-solid-svg-icons/faUndo';
import { faCompressAlt } from '@fortawesome/free-solid-svg-icons/faCompressAlt';
import { faEllipsisV } from '@fortawesome/free-solid-svg-icons/faEllipsisV';
import { Maybe } from 'true-myth';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import useResizeObserver from '@react-hook/resize-observer';
// until ui is moved to its own package this should do it
// eslint-disable-next-line import/no-extraneous-dependencies
import Button from '@webapp/ui/Button';
// eslint-disable-next-line import/no-extraneous-dependencies
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import { Tooltip } from '@pyroscope/webapp/javascript/ui/Tooltip';
import { FitModes, HeadMode, TailMode } from './fitMode/fitMode';
import SharedQueryInput from './SharedQueryInput';
import type { ViewTypes } from './FlameGraph/FlameGraphComponent/viewTypes';
import type { FlamegraphRendererProps } from './FlameGraph/FlameGraphRenderer';
import CheckIcon from './FlameGraph/FlameGraphComponent/CheckIcon';
import {
  TableIcon,
  TablePlusFlamegraphIcon,
  FlamegraphIcon,
  SandwichIcon,
  HeadFirstIcon,
  TailFirstIcon,
} from './Icons';

import styles from './Toolbar.module.scss';

const cx = classNames.bind(styles);

export const TOOLBAR_MODE_WIDTH_THRESHOLD = 900;

export type ShowModeType = ReturnType<typeof useSizeMode>;

// todo: remove this logic
export const useSizeMode = () => {
  return 'large';
};

const QUERY_INPUT_WIDTH = 175;
// 4 is marginLeft(2px) + marginRight(2px). also we can simulate divider side margins here
const TOOLBAR_SQUARE_WIDTH = 40 + 4;
const MORE_BUTTON_WIDTH = 16;

const calculateCollapsedItems = (
  clientWidth: number,
  collapsedItemsNumber: number,
  itemsW: number[]
) => {
  const availableToolbarItemsWidth =
    collapsedItemsNumber === 0
      ? clientWidth - QUERY_INPUT_WIDTH
      : clientWidth - QUERY_INPUT_WIDTH - MORE_BUTTON_WIDTH;

  let collapsedItems = 0;
  let visibleItemsWidth = 0;
  itemsW.forEach((v) => {
    visibleItemsWidth += v;
    if (availableToolbarItemsWidth <= visibleItemsWidth) {
      collapsedItems += 1;
    }
  });

  return collapsedItems;
};

const useMoreButton = (
  target: RefObject<HTMLDivElement>,
  toolbarItemsWidth: number[]
) => {
  const [isCollapsed, setCollapsedStatus] = useState(true);
  const [collapsedItemsNumber, setCollapsedItemsNumber] = useState(0);

  useLayoutEffect(() => {
    if (target.current) {
      const collapsedItems = calculateCollapsedItems(
        target.current.clientWidth,
        collapsedItemsNumber,
        toolbarItemsWidth
      );
      setCollapsedItemsNumber(collapsedItems);
    }
  }, [target.current, toolbarItemsWidth]);

  const handleMoreClick = () => {
    setCollapsedStatus((v) => !v);
  };

  useResizeObserver(target, (entry: ResizeObserverEntry) => {
    const collapsedItems = calculateCollapsedItems(
      entry.target.clientWidth,
      collapsedItemsNumber,
      toolbarItemsWidth
    );

    setCollapsedItemsNumber(collapsedItems);
    setCollapsedStatus(true);
  });

  return {
    isCollapsed,
    handleMoreClick,
    collapsedItemsNumber,
  };
};

export interface ProfileHeaderProps {
  view: ViewTypes;
  enableChangingDisplay?: boolean;
  flamegraphType: 'single' | 'double';
  handleSearchChange: (s: string) => void;
  highlightQuery: string;
  ExportData?: ReactNode;

  /** Whether the flamegraph is different from its original state */
  isFlamegraphDirty: boolean;
  reset: () => void;

  updateFitMode: (f: FitModes) => void;
  fitMode: FitModes;
  updateView: (s: ViewTypes) => void;

  /**
   * Refers to the node that has been selected in the flamegraph
   */
  selectedNode: Maybe<{ i: number; j: number }>;
  onFocusOnSubtree: (i: number, j: number) => void;
  sharedQuery?: FlamegraphRendererProps['sharedQuery'];
}

const Divider = () => <div className={styles.divider} />;

type ToolbarItemType = {
  width: number;
  el: ReactNode;
};

const Toolbar = memo(
  ({
    view,
    handleSearchChange,
    highlightQuery,
    isFlamegraphDirty,
    reset,
    updateFitMode,
    fitMode,
    updateView,
    selectedNode,
    onFocusOnSubtree,
    flamegraphType,
    enableChangingDisplay = true,
    sharedQuery,
    ExportData,
  }: ProfileHeaderProps) => {
    const toolbarRef = useRef<HTMLDivElement>(null);
    const showMode = useSizeMode(toolbarRef);

    const fitModeItem = {
      el: (
        <>
          <FitMode
            showMode={showMode}
            fitMode={fitMode}
            updateFitMode={updateFitMode}
          />
          <Divider />
        </>
      ),
      width: TOOLBAR_SQUARE_WIDTH * 2,
    };
    const resetItem = {
      el: <ResetView isFlamegraphDirty={isFlamegraphDirty} reset={reset} />,
      width: TOOLBAR_SQUARE_WIDTH,
    };
    const focusOnSubtree = {
      el: (
        <>
          <FocusOnSubtree
            selectedNode={selectedNode}
            onFocusOnSubtree={onFocusOnSubtree}
          />
        </>
      ),
      width: TOOLBAR_SQUARE_WIDTH,
    };
    const viewSectionItem = enableChangingDisplay
      ? {
          el: (
            <>
              <Divider />
              <ViewSection
                flamegraphType={flamegraphType}
                showMode={showMode}
                view={view}
                updateView={updateView}
              />
            </>
          ),
          // sandwich view is hidden in diff view
          width: TOOLBAR_SQUARE_WIDTH * (flamegraphType === 'single' ? 4 : 3),
        }
      : null;
    const exportDataItem = isValidElement(ExportData)
      ? {
          el: (
            <>
              <Divider />
              {ExportData}
            </>
          ),
          width: TOOLBAR_SQUARE_WIDTH,
        }
      : null;

    const filteredToolbarItems = [
      fitModeItem,
      resetItem,
      focusOnSubtree,
      viewSectionItem,
      exportDataItem,
    ].filter((v) => v !== null) as ToolbarItemType[];
    const toolbarItemsWidth = filteredToolbarItems.reduce(
      (acc, v) => [...acc, v.width],
      [] as number[]
    );

    const { isCollapsed, collapsedItemsNumber, handleMoreClick } =
      useMoreButton(toolbarRef, toolbarItemsWidth);

    const toolbarFilteredItems = filteredToolbarItems.reduce(
      (acc, v, i, arr) => {
        const isHiddenItem = i > arr.length - 1 - collapsedItemsNumber;

        if (isHiddenItem) {
          acc.hidden.push(v);
        } else {
          acc.visible.push(v);
        }

        return acc;
      },
      { visible: [] as ToolbarItemType[], hidden: [] as ToolbarItemType[] }
    );

    return (
      <div role="toolbar" ref={toolbarRef} data-mode={showMode}>
        <div className={styles.navbar}>
          <div>
            <SharedQueryInput
              width={QUERY_INPUT_WIDTH}
              showMode={showMode}
              onHighlightChange={handleSearchChange}
              highlightQuery={highlightQuery}
              sharedQuery={sharedQuery}
            />
          </div>
          <div>
            <div className={styles.itemsContainer}>
              {toolbarFilteredItems.visible.map((v, i) => (
                // eslint-disable-next-line react/no-array-index-key
                <div key={i} className={styles.item} style={{ width: v.width }}>
                  {v.el}
                </div>
              ))}
              {collapsedItemsNumber !== 0 && (
                <button
                  onClick={handleMoreClick}
                  className={cx({
                    [styles.moreButton]: true,
                    [styles.active]: !isCollapsed,
                  })}
                >
                  <FontAwesomeIcon icon={faEllipsisV} />
                </button>
              )}
            </div>
          </div>
          {!isCollapsed && (
            <div className={styles.navbarCollapsedItems}>
              {toolbarFilteredItems.hidden.map((v, i) => (
                <div
                  // eslint-disable-next-line react/no-array-index-key
                  key={i}
                  className={styles.item}
                  style={{ width: v.width }}
                >
                  {v.el}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }
);

function FocusOnSubtree({
  onFocusOnSubtree,
  selectedNode,
}: {
  selectedNode: ProfileHeaderProps['selectedNode'];
  onFocusOnSubtree: ProfileHeaderProps['onFocusOnSubtree'];
}) {
  const onClick = selectedNode.mapOr(
    () => {},
    (f) => {
      return () => onFocusOnSubtree(f.i, f.j);
    }
  );

  return (
    <Tooltip placement="top" title="Collapse nodes above">
      <div>
        <Button
          disabled={!selectedNode.isJust}
          onClick={onClick}
          className={styles.collapseNodeButton}
          aria-label="Collapse nodes above"
        >
          <FontAwesomeIcon icon={faCompressAlt} />
        </Button>
      </div>
    </Tooltip>
  );
}

function ResetView({
  isFlamegraphDirty,
  reset,
}: {
  isFlamegraphDirty: ProfileHeaderProps['isFlamegraphDirty'];
  reset: ProfileHeaderProps['reset'];
}) {
  return (
    <Tooltip placement="top" title="Reset View">
      <div>
        <Button
          id="reset"
          disabled={!isFlamegraphDirty}
          onClick={reset}
          className={styles.resetViewButton}
          aria-label="Reset View"
        >
          <FontAwesomeIcon icon={faUndo} />
        </Button>
      </div>
    </Tooltip>
  );
}

function FitMode({
  fitMode,
  updateFitMode,
  showMode,
}: {
  showMode: ShowModeType;
  fitMode: ProfileHeaderProps['fitMode'];
  updateFitMode: ProfileHeaderProps['updateFitMode'];
}) {
  let texts = {
    label: '',
    [HeadMode]: '',
    [TailMode]: '',
  };
  let menuButtonClassName = '';
  switch (showMode) {
    case 'small':
      texts = {
        label: 'Fit',
        [HeadMode]: 'Head',
        [TailMode]: 'Tail',
      };
      menuButtonClassName = styles.fitModeDropdownSmall;
      break;
    case 'large':
      texts = {
        label: 'Prefer to Fit',
        [HeadMode]: 'Head first',
        [TailMode]: 'Tail first',
      };
      menuButtonClassName = styles.fitModeDropdownLarge;
      break;
    default: {
      throw new Error('Wrong mode');
    }
  }

  const menuOptions = [HeadMode, TailMode] as FitModes[];
  const menuItems = menuOptions.map((mode) => (
    <MenuItem key={mode} value={mode}>
      <div className={styles.dropdownMenuItem} data-testid={mode}>
        {texts[mode]}
        {fitMode === mode ? <CheckIcon /> : null}
      </div>
    </MenuItem>
  ));

  const isSelected = (a: FitModes) => fitMode === a;

  if (showMode === 'small') {
    return (
      <Tooltip placement="top" title="Fit Mode">
        <div>
          <Dropdown
            label={texts.label}
            ariaLabel="Fit Mode"
            value={texts[fitMode]}
            onItemClick={(event) =>
              updateFitMode(event.value as typeof fitMode)
            }
            menuButtonClassName={menuButtonClassName}
          >
            {menuItems}
          </Dropdown>
        </div>
      </Tooltip>
    );
  }

  return (
    <>
      <Tooltip placement="top" title={texts['HEAD']}>
        <Button
          onClick={() => updateFitMode('HEAD')}
          className={cx({
            [styles.fitModeButton]: true,
            [styles.selected]: isSelected('HEAD'),
          })}
        >
          <HeadFirstIcon />
        </Button>
      </Tooltip>
      <Tooltip placement="top" title={texts['TAIL']}>
        <Button
          onClick={() => updateFitMode('TAIL')}
          className={cx({
            [styles.fitModeButton]: true,
            [styles.selected]: isSelected('TAIL'),
          })}
        >
          <TailFirstIcon />
        </Button>
      </Tooltip>
    </>
  );
}

const getViewOptions = (
  flamegraphType: ProfileHeaderProps['flamegraphType']
): Array<{
  label: string;
  value: ViewTypes;
  Icon: (props: { fill?: string | undefined }) => JSX.Element;
}> =>
  flamegraphType === 'single'
    ? [
        { label: 'Table', value: 'table', Icon: TableIcon },
        {
          label: 'Table and Flamegraph',
          value: 'both',
          Icon: TablePlusFlamegraphIcon,
        },
        {
          label: 'Flamegraph',
          value: 'flamegraph',
          Icon: FlamegraphIcon,
        },
        { label: 'Sandwich', value: 'sandwich', Icon: SandwichIcon },
      ]
    : [
        { label: 'Table', value: 'table', Icon: TableIcon },
        {
          label: 'Table and Flamegraph',
          value: 'both',
          Icon: TablePlusFlamegraphIcon,
        },
        {
          label: 'Flamegraph',
          value: 'flamegraph',
          Icon: FlamegraphIcon,
        },
      ];

function ViewSection({
  view,
  updateView,
  showMode,
  flamegraphType,
}: {
  showMode: ShowModeType;
  updateView: ProfileHeaderProps['updateView'];
  view: ProfileHeaderProps['view'];
  flamegraphType: ProfileHeaderProps['flamegraphType'];
}) {
  const options = getViewOptions(flamegraphType);

  const dropdownMenuItems = options.map((mode) => (
    <MenuItem key={mode.value} value={mode.value}>
      <div className={styles.dropdownMenuItem} data-testid={mode.value}>
        {mode.label}
        {view === mode.value ? <CheckIcon /> : null}
      </div>
    </MenuItem>
  ));

  const ViewSelect = (
    <Tooltip placement="top" title="View Mode">
      <div>
        <Dropdown
          label="View Mode"
          ariaLabel="View Mode"
          value={options.find((i) => i.value === view)?.label}
          onItemClick={(event) => updateView(event.value)}
          align="end"
          menuButtonClassName={styles.viewModeDropdownButton}
        >
          {dropdownMenuItems}
        </Dropdown>
      </div>
    </Tooltip>
  );

  const ViewButtons = options.map(({ label, value, Icon }) => (
    <Tooltip key={value} placement="top" title={label}>
      <Button
        onClick={() => updateView(value)}
        className={cx({
          [styles.toggleViewButton]: true,
          selected: view === value,
        })}
      >
        <Icon />
      </Button>
    </Tooltip>
  ));

  const decideWhatToShow = () => {
    switch (showMode) {
      case 'small': {
        return ViewSelect;
      }
      case 'large': {
        return ViewButtons;
      }

      default: {
        throw new Error(`Invalid option: '${showMode}'`);
      }
    }
  };

  return <div className={styles.viewType}>{decideWhatToShow()}</div>;
}

export default Toolbar;
