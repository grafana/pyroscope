import React, { ReactNode } from 'react';
import classNames from 'classnames/bind';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { faListUl } from '@fortawesome/free-solid-svg-icons/faListUl';
import { faUndo } from '@fortawesome/free-solid-svg-icons/faUndo';
import { faCompressAlt } from '@fortawesome/free-solid-svg-icons/faCompressAlt';
import { Maybe } from 'true-myth';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import useResizeObserver from '@react-hook/resize-observer';
// until ui is moved to its own package this should do it
// eslint-disable-next-line import/no-extraneous-dependencies
import Button from '@webapp/ui/Button';
// eslint-disable-next-line import/no-extraneous-dependencies
import Select from '@webapp/ui/Select';
// eslint-disable-next-line import/no-extraneous-dependencies
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
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
import { Tooltip } from '@pyroscope/webapp/javascript/ui/Tooltip';

import styles from './Toolbar.module.scss';

const cx = classNames.bind(styles);

/**
 * Custom hook that returns the size ('large' | 'small')
 * that should be displayed
 * based on the toolbar width
 */
export type ShowModeType = ReturnType<typeof useSizeMode>;

export const useSizeMode = (
  target: React.RefObject<HTMLDivElement>,
  // arbitrary value
  // as a simple heuristic, try to run the comparison view
  // and see when the buttons start to overlap
  widthTreshhold: number
) => {
  const [size, setSize] = React.useState<'large' | 'small'>('large');

  const calcMode = (width: number) => {
    if (width < widthTreshhold) {
      return 'small';
    }
    return 'large';
  };

  React.useLayoutEffect(() => {
    if (target.current) {
      const { width } = target.current.getBoundingClientRect();

      setSize(calcMode(width));
    }
  }, [target.current]);

  useResizeObserver(target, (entry: ResizeObserverEntry) => {
    setSize(calcMode(entry.contentRect.width));
  });

  return size;
};

const useShowMode = (widthTreshhold) => {
  // TODO: merge useShowMode and useSizeMode
  const ref = React.useRef<HTMLDivElement>(null);
  const showMode = useSizeMode(ref, widthTreshhold);

  return {
    ref,
    showMode,
  };
};

export interface ProfileHeaderProps {
  view: ViewTypes;
  disableChangingDisplay?: boolean;
  flamegraphType: 'single' | 'double';
  viewDiff: 'diff' | 'total' | 'self';
  handleSearchChange: (s: string) => void;
  highlightQuery: string;
  ExportData?: ReactNode;

  /** Whether the flamegraph is different from its original state */
  isFlamegraphDirty: boolean;
  reset: () => void;

  updateFitMode: (f: FitModes) => void;
  fitMode: FitModes;
  updateView: (s: ViewTypes) => void;
  updateViewDiff: (s: 'diff' | 'total' | 'self') => void;

  /**
   * Refers to the node that has been selected in the flamegraph
   */
  selectedNode: Maybe<{ i: number; j: number }>;
  onFocusOnSubtree: (i: number, j: number) => void;
  sharedQuery?: FlamegraphRendererProps['sharedQuery'];
  panesOrientation: 'horizontal' | 'vertical';
}

const Divider = () => <div className={styles.divider} />;

const Toolbar = React.memo(
  ({
    view,
    viewDiff,
    handleSearchChange,
    highlightQuery,
    isFlamegraphDirty,
    reset,
    updateFitMode,
    fitMode,
    updateView,
    updateViewDiff,
    selectedNode,
    onFocusOnSubtree,
    flamegraphType,
    disableChangingDisplay = false,
    sharedQuery,
    ExportData = <></>,
    panesOrientation,
  }: ProfileHeaderProps) => {
    return (
      <div role="toolbar">
        <div className={styles.navbar}>
          <LeftToolbar
            handleSearchChange={handleSearchChange}
            highlightQuery={highlightQuery}
            sharedQuery={sharedQuery}
            flamegraphType={flamegraphType}
            viewDiff={viewDiff}
            updateViewDiff={updateViewDiff}
            panesOrientation={panesOrientation}
          />
          <RightToolbar
            // showMode={showMode}
            fitMode={fitMode}
            updateFitMode={updateFitMode}
            isFlamegraphDirty={isFlamegraphDirty}
            reset={reset}
            selectedNode={selectedNode}
            onFocusOnSubtree={onFocusOnSubtree}
            disableChangingDisplay={disableChangingDisplay}
            view={view}
            updateView={updateView}
            ExportData={ExportData}
            panesOrientation={panesOrientation}
          />
        </div>
      </div>
    );
  }
);

const LeftToolbar = ({
  handleSearchChange,
  highlightQuery,
  sharedQuery,
  flamegraphType,
  viewDiff,
  updateViewDiff,
  panesOrientation,
}) => {
  const { ref, showMode } = useShowMode(500);

  return (
    <div
      ref={ref}
      className={cx({
        [styles.left]: true,
        widthAuto: panesOrientation === 'vertical',
      })}
    >
      <SharedQueryInput
        showMode={showMode}
        onHighlightChange={handleSearchChange}
        highlightQuery={highlightQuery}
        sharedQuery={sharedQuery}
      />
      {flamegraphType === 'double' && (
        <DiffView
          showMode={showMode}
          viewDiff={viewDiff}
          updateViewDiff={updateViewDiff}
        />
      )}
    </div>
  );
};

export const RightToolbar = ({
  fitMode,
  updateFitMode,
  isFlamegraphDirty,
  reset,
  selectedNode,
  onFocusOnSubtree,
  disableChangingDisplay,
  view,
  updateView,
  ExportData,
  panesOrientation,
}) => {
  const { ref, showMode } = useShowMode(
    panesOrientation === 'vertical' ? 491 : 480
  );

  return (
    <div
      ref={ref}
      className={cx({
        [styles.right]: true,
        widthAuto: panesOrientation === 'vertical',
      })}
    >
      <FitMode
        showMode={showMode}
        fitMode={fitMode}
        updateFitMode={updateFitMode}
      />
      <Divider />
      <ResetView isFlamegraphDirty={isFlamegraphDirty} reset={reset} />
      <FocusOnSubtree
        selectedNode={selectedNode}
        onFocusOnSubtree={onFocusOnSubtree}
      />
      <Divider />
      {!disableChangingDisplay && (
        <ViewSection showMode={showMode} view={view} updateView={updateView} />
      )}
      <Divider />
      {ExportData}
    </div>
  );
};

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
          data-testid="reset-view"
          disabled={!isFlamegraphDirty}
          onClick={reset}
          className={styles.resetViewButton}
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
  showMode: ReturnType<typeof useSizeMode>;
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
      <div className={styles.fitModeDropdownMenuItem} data-testid={mode}>
        {texts[mode]}
        {fitMode === mode ? <CheckIcon /> : null}
      </div>
    </MenuItem>
  ));

  const isSelected = (a) => fitMode === a;

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
      <Tooltip placement="top" title="Head First">
        <Button
          onClick={() => updateFitMode('HEAD')}
          className={cx({
            [styles.fitModeButton]: true,
            selected: isSelected('HEAD'),
          })}
        >
          <HeadFirstIcon />
        </Button>
      </Tooltip>
      <Tooltip placement="top" title="Tail First">
        <Button
          onClick={() => updateFitMode('TAIL')}
          className={cx({
            [styles.fitModeButton]: true,
            selected: isSelected('TAIL'),
          })}
        >
          <TailFirstIcon />
        </Button>
      </Tooltip>
    </>
  );
}

function DiffView({
  viewDiff,
  updateViewDiff,
  showMode,
}: {
  showMode: ReturnType<typeof useSizeMode>;
  updateViewDiff: ProfileHeaderProps['updateViewDiff'];
  viewDiff: ProfileHeaderProps['viewDiff'];
}) {
  if (!viewDiff) {
    return null;
  }

  const ShowModeSelect = (
    <Select
      name="viewDiff"
      ariaLabel="view-diff"
      value={viewDiff}
      onChange={(e) => {
        updateViewDiff(e.target.value as typeof viewDiff);
      }}
    >
      <option value="self">Self</option>
      <option value="total">Total</option>
      <option value="diff">Diff</option>
    </Select>
  );

  const kindByState = (name: string) => {
    if (viewDiff === name) {
      return 'primary';
    }
    return 'default';
  };

  const Buttons = (
    <>
      <Button
        grouped
        icon={faListUl}
        kind={kindByState('self')}
        onClick={() => updateViewDiff('self')}
      >
        Self
      </Button>
      <Button
        grouped
        icon={faBars}
        kind={kindByState('total')}
        onClick={() => updateViewDiff('total')}
      >
        Total
      </Button>
      <Button
        grouped
        icon={faAlignLeft}
        kind={kindByState('diff')}
        onClick={() => updateViewDiff('diff')}
      >
        Diff
      </Button>
    </>
  );

  const decideWhatToShow = () => {
    switch (showMode) {
      case 'small': {
        return ShowModeSelect;
      }
      case 'large': {
        return Buttons;
      }

      default: {
        throw new Error(`Invalid option: '${showMode}'`);
      }
    }
  };

  return (
    <div className="btn-group" data-testid="diff-view">
      {decideWhatToShow()}
    </div>
  );
}

function ViewSection({
  view,
  updateView,
  showMode,
}: {
  showMode: ReturnType<typeof useSizeMode>;
  updateView: ProfileHeaderProps['updateView'];
  view: ProfileHeaderProps['view'];
}) {
  const ViewSelect = (
    <Tooltip placement="top" title="View Mode">
      <div>
        <Select
          ariaLabel="view"
          name="view"
          value={view}
          onChange={(e) => {
            updateView(e.target.value as typeof view);
          }}
          className={styles.showModeSelect}
        >
          <option value="table">Table</option>
          <option value="both">Both</option>
          <option value="flamegraph">Flame</option>
          <option value="sandwich">Sandwich</option>
        </Select>
      </div>
    </Tooltip>
  );

  const isSelected = (name: ViewTypes) => view === name;

  const Buttons = (
    <>
      <Tooltip placement="top" title="Table View">
        <Button
          onClick={() => updateView('table')}
          className={cx({
            [styles.toggleViewButton]: true,
            selected: isSelected('table'),
          })}
        >
          <TableIcon />
        </Button>
      </Tooltip>
      <Tooltip placement="top" title="Both View">
        <Button
          onClick={() => updateView('both')}
          className={cx({
            [styles.toggleViewButton]: true,
            selected: isSelected('both'),
          })}
        >
          <TablePlusFlamegraphIcon />
        </Button>
      </Tooltip>
      <Tooltip placement="top" title="Flamegraph View">
        <Button
          onClick={() => updateView('flamegraph')}
          className={cx({
            [styles.toggleViewButton]: true,
            selected: isSelected('flamegraph'),
          })}
        >
          <FlamegraphIcon />
        </Button>
      </Tooltip>
      <Tooltip placement="top" title="Sandwich View">
        <Button
          onClick={() => updateView('sandwich')}
          className={cx({
            [styles.toggleViewButton]: true,
            selected: isSelected('sandwich'),
          })}
        >
          <SandwichIcon />
        </Button>
      </Tooltip>
    </>
  );

  const decideWhatToShow = () => {
    switch (showMode) {
      case 'small': {
        return ViewSelect;
      }
      case 'large': {
        return Buttons;
      }

      default: {
        throw new Error(`Invalid option: '${showMode}'`);
      }
    }
  };

  return <div className={styles.viewType}>{decideWhatToShow()}</div>;
}

export default Toolbar;
