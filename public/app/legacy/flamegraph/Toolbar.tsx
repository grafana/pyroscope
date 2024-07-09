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
import Button from '@pyroscope/ui/Button';
// eslint-disable-next-line import/no-extraneous-dependencies
import { Tooltip } from '@pyroscope/ui/Tooltip';
import { FitModes } from './fitMode/fitMode';
import SharedQueryInput from './SharedQueryInput';
import type { ViewTypes } from './FlameGraph/FlameGraphComponent/viewTypes';
import type { FlamegraphRendererProps } from './FlameGraph/FlameGraphRenderer';
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

const DIVIDER_WIDTH = 5;
const QUERY_INPUT_WIDTH = 175;
const LEFT_MARGIN = 2;
const RIGHT_MARGIN = 2;
const TOOLBAR_SQUARE_WIDTH = 40 + LEFT_MARGIN + RIGHT_MARGIN;
const MORE_BUTTON_WIDTH = 16;

const calculateCollapsedItems = (
  clientWidth: number,
  collapsedItemsNumber: number,
  itemsW: number[]
) => {
  const availableToolbarItemsWidth =
    collapsedItemsNumber === 0
      ? clientWidth - QUERY_INPUT_WIDTH - 5
      : clientWidth - QUERY_INPUT_WIDTH - MORE_BUTTON_WIDTH - 5;

  let collapsedItems = 0;
  let visibleItemsWidth = 0;
  itemsW.reverse().forEach((v) => {
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

  const currentTarget = target.current;

  useLayoutEffect(() => {
    if (currentTarget) {
      const { width } = currentTarget.getBoundingClientRect();
      const collapsedItems = calculateCollapsedItems(
        width,
        collapsedItemsNumber,
        toolbarItemsWidth
      );
      setCollapsedItemsNumber(collapsedItems);
    }
  }, [currentTarget, toolbarItemsWidth, collapsedItemsNumber]);

  const handleMoreClick = () => {
    setCollapsedStatus((v) => !v);
  };

  useResizeObserver(target, (entry: ResizeObserverEntry) => {
    const { width } = entry.target.getBoundingClientRect();
    const collapsedItems = calculateCollapsedItems(
      width,
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

    const fitModeItem = {
      el: (
        <>
          <FitMode fitMode={fitMode} updateFitMode={updateFitMode} />
          <Divider />
        </>
      ),
      width: TOOLBAR_SQUARE_WIDTH * 2 + DIVIDER_WIDTH,
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
          <Divider />
        </>
      ),
      width: TOOLBAR_SQUARE_WIDTH + DIVIDER_WIDTH,
    };

    const viewSectionItem = enableChangingDisplay
      ? {
          el: (
            <ViewSection
              flamegraphType={flamegraphType}
              view={view}
              updateView={updateView}
            />
          ),
          // sandwich view is hidden in diff view
          // Note that  that the toolbar sections width is hardcoded here in terms of the number of buttons expected -- 4x for one condition, and 3x for the other.
          width: TOOLBAR_SQUARE_WIDTH * (flamegraphType === 'single' ? 4 : 3), // 1px is to display divider
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
          width: TOOLBAR_SQUARE_WIDTH + DIVIDER_WIDTH,
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
      (acc, v, i) => {
        const isHiddenItem = i < collapsedItemsNumber;

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
      <div role="toolbar" ref={toolbarRef}>
        <div className={styles.navbar}>
          <div>
            <SharedQueryInput
              width={QUERY_INPUT_WIDTH}
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
                <Tooltip placement="top" title="More">
                  <button
                    onClick={handleMoreClick}
                    className={cx({
                      [styles.moreButton]: true,
                      [styles.active]: !isCollapsed,
                    })}
                  >
                    <FontAwesomeIcon icon={faEllipsisV} />
                  </button>
                </Tooltip>
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
Toolbar.displayName = 'Toolbar';

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
      <span>
        <Button
          id="reset"
          disabled={!isFlamegraphDirty}
          onClick={reset}
          className={styles.resetViewButton}
          aria-label="Reset View"
        >
          <FontAwesomeIcon icon={faUndo} />
        </Button>
      </span>
    </Tooltip>
  );
}

function FitMode({
  fitMode,
  updateFitMode,
}: {
  fitMode: ProfileHeaderProps['fitMode'];
  updateFitMode: ProfileHeaderProps['updateFitMode'];
}) {
  const isSelected = (a: FitModes) => fitMode === a;

  return (
    <>
      <Tooltip placement="top" title="Head first">
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
      <Tooltip placement="top" title="Tail first">
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
  flamegraphType,
}: {
  updateView: ProfileHeaderProps['updateView'];
  view: ProfileHeaderProps['view'];
  flamegraphType: ProfileHeaderProps['flamegraphType'];
}) {
  const options = getViewOptions(flamegraphType);

  return (
    <div className={styles.viewType}>
      {options.map(({ label, value, Icon }) => (
        <Tooltip key={value} placement="top" title={label}>
          <Button
            data-testid={value}
            onClick={() => updateView(value)}
            className={cx({
              [styles.toggleViewButton]: true,
              selected: view === value,
            })}
          >
            <Icon />
          </Button>
        </Tooltip>
      ))}
    </div>
  );
}

export default Toolbar;
