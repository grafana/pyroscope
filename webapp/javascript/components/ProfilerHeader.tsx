import React, { useCallback } from 'react';
import clsx from 'clsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faIcicles } from '@fortawesome/free-solid-svg-icons/faIcicles';
import { faListUl } from '@fortawesome/free-solid-svg-icons/faListUl';
import { faTable } from '@fortawesome/free-solid-svg-icons/faTable';
import { debounce } from 'lodash';
import useResizeObserver from '@react-hook/resize-observer';
import { Option } from 'prelude-ts';
import { FitModes } from '../util/fitMode';
import styles from './ProfilerHeader.module.css';

// arbitrary value
// as a simple heuristic, try to run the comparison view
// and see when the buttons start to overlap
export const TOOLBAR_MODE_WIDTH_THRESHOLD = 900;

// items may have 2 modes: large and small
type ItemMode = 'large' | 'small';

/**
 * Custom hook that returns the ItemMode
 * that should be displayed
 * based on the toolbar width
 */
const useSizeMode = (target: React.RefObject<HTMLDivElement>) => {
  const [size, setSize] = React.useState<ItemMode>('large');

  const calcMode = (width: number) => {
    if (width < TOOLBAR_MODE_WIDTH_THRESHOLD) {
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

interface ProfileHeaderProps {
  view: 'both' | 'icicle' | 'table';
  viewDiff?: 'diff' | 'total' | 'self';
  handleSearchChange: (s: string) => void;

  /** Whether the flamegraph is different from its original state */
  isFlamegraphDirty: boolean;
  reset: () => void;

  updateFitMode: (f: FitModes) => void;
  fitMode: FitModes;
  updateView: (s: 'both' | 'icicle' | 'table') => void;
  updateViewDiff: (s: 'diff' | 'total' | 'self') => void;

  /**
   * Refers to the node that has been selected in the flamegraph
   */
  selectedNode: Option<{ i: number; j: number }>;
  onFocusOnSubtree: (node: { i: number; j: number }) => void;
}

const ProfilerHeader = React.memo(
  ({
    view,
    viewDiff,
    handleSearchChange,
    isFlamegraphDirty,
    reset,
    updateFitMode,
    fitMode,
    updateView,
    updateViewDiff,

    selectedNode,
    onFocusOnSubtree,
  }: ProfileHeaderProps) => {
    const toolbarRef = React.useRef();
    const showMode = useSizeMode(toolbarRef);

    return (
      <div role="toolbar" ref={toolbarRef} data-mode={showMode}>
        <div className="navbar-2">
          <HighlightSearch onHighlightChange={handleSearchChange} />
          &nbsp;
          <ResetView isFlamegraphDirty={isFlamegraphDirty} reset={reset} />
          <FitMode fitMode={fitMode} updateFitMode={updateFitMode} />
          <FocusOnSubtree
            selectedNode={selectedNode}
            onFocusOnSubtree={onFocusOnSubtree}
          />
          <div className="navbar-space-filler" />
          <DiffView
            showMode={showMode}
            viewDiff={viewDiff}
            updateViewDiff={updateViewDiff}
          />
          <ViewSection
            showMode={showMode}
            view={view}
            updateView={updateView}
          />
        </div>
      </div>
    );
  }
);

function FocusOnSubtree({ onFocusOnSubtree, selectedNode }) {
  const f = selectedNode;
  const onClick = f.isNone()
    ? () => {}
    : () => {
        onFocusOnSubtree(f.get().i, f.get().j);
      };

  return (
    <button
      className={clsx('btn')}
      disabled={!selectedNode.isSome()}
      onClick={onClick}
    >
      Focus on Subtree
    </button>
  );
}

function HighlightSearch({ onHighlightChange }) {
  // debounce the search
  // since rebuilding the canvas on each keystroke is expensive
  const deb = useCallback(
    debounce(
      (s: string) => {
        onHighlightChange(s);
      },
      250,
      { maxWait: 1000 }
    ),
    []
  );

  const onChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const q = e.target.value;
    deb(q);
  };

  return (
    <input
      type="search"
      data-testid="flamegraph-search"
      className="flamegraph-search"
      name="flamegraph-search"
      placeholder="Search…"
      onChange={onChange}
    />
  );
}

function ResetView({ isFlamegraphDirty, reset }) {
  return (
    <button
      type="button"
      className={clsx('btn')}
      disabled={!isFlamegraphDirty}
      data-testid="reset-view"
      id="reset"
      onClick={reset}
    >
      Reset View
    </button>
  );
}

function FitMode({ fitMode, updateFitMode }) {
  return (
    <select
      aria-label="fit-mode"
      className="fit-mode-select"
      value={fitMode}
      onChange={(event) => updateFitMode(event.target.value)}
    >
      <option disabled>Prefer to fit</option>
      <option value={FitModes.HEAD}>Head First</option>
      <option value={FitModes.TAIL}>Tail First</option>
    </select>
  );
}

function DiffView({ viewDiff, updateViewDiff, showMode }) {
  if (!viewDiff) {
    return null;
  }

  const Select = (
    <select
      aria-label="view-diff"
      value={viewDiff}
      onChange={(e) => {
        updateViewDiff(e.target.value);
      }}
    >
      <option value="self">Self</option>
      <option value="total">Total</option>
      <option value="diff">Diff</option>
    </select>
  );

  const Buttons = (
    <>
      <button
        type="button"
        className={clsx('btn', { active: viewDiff === 'self' })}
        onClick={() => updateViewDiff('self')}
      >
        <FontAwesomeIcon icon={faListUl} />
        &nbsp;&thinsp;Self
      </button>
      <button
        type="button"
        className={clsx('btn', { active: viewDiff === 'total' })}
        onClick={() => updateViewDiff('total')}
      >
        <FontAwesomeIcon icon={faBars} />
        &nbsp;&thinsp;Total
      </button>
      <button
        type="button"
        className={clsx('btn', { active: viewDiff === 'diff' })}
        onClick={() => updateViewDiff('diff')}
      >
        <FontAwesomeIcon icon={faAlignLeft} />
        &nbsp;&thinsp;Diff
      </button>
    </>
  );

  const decideWhatToShow = () => {
    switch (showMode) {
      case 'small': {
        return Select;
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
    <div className="btn-group viz-switch" data-testid="diff-view">
      {decideWhatToShow()}
    </div>
  );
}

function ViewSection({ view, updateView, showMode }) {
  const Select = (
    <select
      aria-label="view"
      value={view}
      onChange={(e) => {
        updateView(e.target.value);
      }}
    >
      <option value="table">Table</option>
      <option value="both">Both</option>
      <option value="icicle">Flamegraph</option>
    </select>
  );

  const Buttons = (
    <>
      <button
        type="button"
        data-testid="btn-table-view"
        className={clsx('btn', { active: view === 'table' })}
        onClick={() => updateView('table')}
      >
        <FontAwesomeIcon icon={faTable} />
        &nbsp;&thinsp;Table
      </button>
      <button
        data-testid="btn-both-view"
        type="button"
        className={clsx('btn', { active: view === 'both' })}
        onClick={() => updateView('both')}
      >
        <FontAwesomeIcon icon={faColumns} />
        &nbsp;&thinsp;Both
      </button>
      <button
        data-testid="btn-flamegraph-view"
        type="button"
        className={clsx('btn', { active: view === 'icicle' })}
        onClick={() => updateView('icicle')}
      >
        <FontAwesomeIcon icon={faIcicles} />
        &nbsp;&thinsp;Flamegraph
      </button>
    </>
  );

  const decideWhatToShow = () => {
    switch (showMode) {
      case 'small': {
        return Select;
      }
      case 'large': {
        return Buttons;
      }

      default: {
        throw new Error(`Invalid option: '${showMode}'`);
      }
    }
  };

  return <div className="btn-group viz-switch">{decideWhatToShow()}</div>;
}

export default ProfilerHeader;
