import React, { ReactNode } from 'react';
import classNames from 'classnames/bind';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { faListUl } from '@fortawesome/free-solid-svg-icons/faListUl';
import { faUndo } from '@fortawesome/free-solid-svg-icons/faUndo';
import { faCompressAlt } from '@fortawesome/free-solid-svg-icons/faCompressAlt';
import { IconDefinition } from '@fortawesome/fontawesome-common-types';
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

export type ShowModeType = 'large' | 'small';

const useShowMode = (widthTreshhold: number) => {
  const [size, setSize] = React.useState<'large' | 'small'>('large');
  const ref = React.useRef<HTMLDivElement>(null);

  const calcMode = (width: number) => {
    if (width < widthTreshhold) {
      return 'small';
    }
    return 'large';
  };

  React.useLayoutEffect(() => {
    if (ref.current) {
      const { width } = ref.current.getBoundingClientRect();

      setSize(calcMode(width));
    }
  }, [ref.current]);

  useResizeObserver(ref, (entry: ResizeObserverEntry) => {
    setSize(calcMode(entry.contentRect.width));
  });

  return {
    ref,
    size,
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
            view={view}
          />
          <RightToolbar
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
  view,
}: Pick<
  ProfileHeaderProps,
  | 'handleSearchChange'
  | 'highlightQuery'
  | 'sharedQuery'
  | 'flamegraphType'
  | 'viewDiff'
  | 'updateViewDiff'
  | 'panesOrientation'
  | 'view'
>) => {
  const { ref, size } = useShowMode(480);

  return (
    <div
      ref={ref}
      className={cx({
        [styles.left]: true,
        [styles.widthAuto]: panesOrientation === 'vertical',
        [styles.merged]: view === 'flamegraph' || view === 'table',
      })}
    >
      <SharedQueryInput
        showMode={size}
        onHighlightChange={handleSearchChange}
        highlightQuery={highlightQuery}
        sharedQuery={sharedQuery}
      />
      {flamegraphType === 'double' && (
        <DiffView
          showMode={size}
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
}: Pick<
  ProfileHeaderProps,
  | 'fitMode'
  | 'updateFitMode'
  | 'isFlamegraphDirty'
  | 'reset'
  | 'selectedNode'
  | 'onFocusOnSubtree'
  | 'disableChangingDisplay'
  | 'view'
  | 'updateView'
  | 'ExportData'
  | 'panesOrientation'
>) => {
  const { ref, size } = useShowMode(
    panesOrientation === 'vertical' ? 491 : 480
  );

  return (
    <div
      ref={ref}
      className={cx({
        [styles.right]: true,
        [styles.widthAuto]: panesOrientation === 'vertical',
        [styles.merged]: view === 'flamegraph' || view === 'table',
      })}
    >
      <FitMode
        showMode={size}
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
        <ViewSection showMode={size} view={view} updateView={updateView} />
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

function DiffView({
  viewDiff,
  updateViewDiff,
  showMode,
}: {
  showMode: ShowModeType;
  updateViewDiff: ProfileHeaderProps['updateViewDiff'];
  viewDiff: ProfileHeaderProps['viewDiff'];
}) {
  if (!viewDiff) {
    return null;
  }

  const diffTypes: Array<{
    value: ProfileHeaderProps['viewDiff'];
    label: string;
    icon: IconDefinition;
  }> = [
    { value: 'self', label: 'Self', icon: faListUl },
    { value: 'total', label: 'Total', icon: faBars },
    { value: 'diff', label: 'Diff', icon: faAlignLeft },
  ];

  const dropdownMenuItems = diffTypes.map((mode) => (
    <MenuItem key={mode.value} value={mode.value}>
      <div className={styles.dropdownMenuItem} data-testid={mode.value}>
        {mode.label.split(' ')[0]}
        {viewDiff === mode.value ? <CheckIcon /> : null}
      </div>
    </MenuItem>
  ));

  const DiffSelect = (
    <Tooltip placement="top" title="Diff View">
      <div>
        <Dropdown
          label="Diff View"
          ariaLabel="Diff View"
          value={viewDiff}
          onItemClick={(event) => updateViewDiff(event.value)}
          align="center"
          menuButtonClassName={styles.diffDropdownButton}
        >
          {dropdownMenuItems}
        </Dropdown>
      </div>
    </Tooltip>
  );

  const DiffButtons = diffTypes.map(({ label, value, icon }) => {
    return (
      <Tooltip key={value} placement="top" title={label}>
        <Button
          onClick={() => updateViewDiff(value)}
          className={cx({
            [styles.toggleViewButton]: true,
            [styles.diffTypesButton]: true,
            selected: viewDiff === value,
          })}
        >
          <FontAwesomeIcon icon={icon} />
        </Button>
      </Tooltip>
    );
  });

  const decideWhatToShow = () => {
    switch (showMode) {
      case 'small': {
        return DiffSelect;
      }
      case 'large': {
        return DiffButtons;
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
  showMode: ShowModeType;
  updateView: ProfileHeaderProps['updateView'];
  view: ProfileHeaderProps['view'];
}) {
  const options: Array<{
    label: string;
    value: ViewTypes;
    Icon: (props: { fill?: string | undefined }) => JSX.Element;
  }> = [
    { label: 'Table View', value: 'table', Icon: TableIcon },
    { label: 'Both View', value: 'both', Icon: TablePlusFlamegraphIcon },
    { label: 'Flamegraph View', value: 'flamegraph', Icon: FlamegraphIcon },
    { label: 'Sandwich View', value: 'sandwich', Icon: SandwichIcon },
  ];

  const dropdownMenuItems = options.map((mode) => (
    <MenuItem key={mode.value} value={mode.value}>
      <div className={styles.dropdownMenuItem} data-testid={mode.value}>
        {mode.label.split(' ')[0]}
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
          value={options.find((i) => i.value === view)?.label?.split(' ')[0]}
          onItemClick={(event) => updateView(event.value)}
          align="center"
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
