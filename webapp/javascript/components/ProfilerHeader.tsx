import React, { useCallback } from 'react';
import clsx from 'clsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faIcicles } from '@fortawesome/free-solid-svg-icons/faIcicles';
import { faListUl } from '@fortawesome/free-solid-svg-icons/faListUl';
import { faTable } from '@fortawesome/free-solid-svg-icons/faTable';
import { faUndo } from '@fortawesome/free-solid-svg-icons/faUndo';
import { faCompressAlt } from '@fortawesome/free-solid-svg-icons/faCompressAlt';
import { DebounceInput } from 'react-debounce-input';
import { Option } from 'prelude-ts';
import useResizeObserver from '@react-hook/resize-observer';
import { FitModes } from '../util/fitMode';
import styles from './ProfilerHeader.module.css';

// arbitrary value
// as a simple heuristic, try to run the comparison view
// and see when the buttons start to overlap
export const TOOLBAR_MODE_WIDTH_THRESHOLD = 900;

/**
 * Custom hook that returns the size ('large' | 'small')
 * that should be displayed
 * based on the toolbar width
 */
const useSizeMode = (target: React.RefObject<HTMLDivElement>) => {
  const [size, setSize] = React.useState<'large' | 'small'>('large');

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
        <div className={styles.navbar}>
          <HighlightSearch
            showMode={showMode}
            onHighlightChange={handleSearchChange}
          />
          <DiffView
            showMode={showMode}
            viewDiff={viewDiff}
            updateViewDiff={updateViewDiff}
          />
          <div className={styles['space-filler']} />
          <FitMode fitMode={fitMode} updateFitMode={updateFitMode} />
          <ResetView
            showMode={showMode}
            isFlamegraphDirty={isFlamegraphDirty}
            reset={reset}
          />
          <FocusOnSubtree
            showMode={showMode}
            selectedNode={selectedNode}
            onFocusOnSubtree={onFocusOnSubtree}
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

function FocusOnSubtree({ onFocusOnSubtree, selectedNode, showMode }) {
  let text = '';
  switch (showMode) {
    case 'small': {
      text = 'Focus';
      break;
    }
    case 'large': {
      text = 'Focus on subtree';
      break;
    }

    default:
      throw new Error('Wrong mode');
  }

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
      <FontAwesomeIcon icon={faCompressAlt} />
      &nbsp;&thinsp;{text}
    </button>
  );
}

function HighlightSearch({ onHighlightChange, showMode }) {
  return (
    <DebounceInput
      data-testid="flamegraph-search"
      className={`${styles.search} ${
        showMode === 'small' ? styles['search-small'] : ''
      }`}
      type="search"
      name="flamegraph-search"
      placeholder="Search…"
      minLength={2}
      debounceTimeout={100}
      onChange={(e) => {
        onHighlightChange(e.target.value);
      }}
    />
  );
}

function ResetView({ isFlamegraphDirty, reset, showMode }) {
  let text = '';
  switch (showMode) {
    case 'small': {
      text = 'Reset';
      break;
    }
    case 'large': {
      text = 'Reset View';
      break;
    }

    default:
      throw new Error('Wrong mode');
  }
  return (
    <button
      type="button"
      className={clsx('btn')}
      disabled={!isFlamegraphDirty}
      data-testid="reset-view"
      id="reset"
      onClick={reset}
    >
      <FontAwesomeIcon icon={faUndo} />
      &nbsp;&thinsp;{text}
    </button>
  );
}

function FitMode({ fitMode, updateFitMode }) {
  return (
    <select
      aria-label="fit-mode"
      className={styles['fit-mode-select']}
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
        className={`${clsx('btn', {
          active: viewDiff === 'self',
        })} ${styles['visualization-buttons']} `}
        onClick={() => updateViewDiff('self')}
      >
        <FontAwesomeIcon icon={faListUl} />
        &nbsp;&thinsp;Self
      </button>
      <button
        type="button"
        className={`${clsx('btn', {
          active: viewDiff === 'total',
        })} ${styles['visualization-buttons']} `}
        onClick={() => updateViewDiff('total')}
      >
        <FontAwesomeIcon icon={faBars} />
        &nbsp;&thinsp;Total
      </button>
      <button
        type="button"
        className={`${clsx('btn', {
          active: viewDiff === 'diff',
        })} ${styles['visualization-buttons']} `}
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
        className={`${clsx('btn', {
          active: view === 'table',
        })} ${styles['visualization-buttons']} `}
        onClick={() => updateView('table')}
      >
        <FontAwesomeIcon icon={faTable} />
        &nbsp;&thinsp;Table
      </button>
      <button
        data-testid="btn-both-view"
        type="button"
        className={`${clsx('btn', {
          active: view === 'both',
        })} ${styles['visualization-buttons']} `}
        onClick={() => updateView('both')}
      >
        <FontAwesomeIcon icon={faColumns} />
        &nbsp;&thinsp;Both
      </button>
      <button
        data-testid="btn-flamegraph-view"
        type="button"
        className={`${clsx('btn', {
          active: view === 'icicle',
        })} ${styles['visualization-buttons']} `}
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
