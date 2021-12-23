import React from 'react';
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
import { Maybe } from '@utils/fp';
import useResizeObserver from '@react-hook/resize-observer';
import Button from '@ui/Button';
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
  // what's being displayed
  // this is needed since the toolbar may show different items depending what is being displayed
  display: 'flamegraph' | 'table' | 'both';

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
  selectedNode: Maybe<{ i: number; j: number }>;
  onFocusOnSubtree: (i: number, j: number) => void;
}

const Toolbar = React.memo(
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
    display,

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
          <FitMode
            showMode={showMode}
            fitMode={fitMode}
            updateFitMode={updateFitMode}
          />
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
          {display === 'both' && (
            <ViewSection
              showMode={showMode}
              view={view}
              updateView={updateView}
            />
          )}
        </div>
      </div>
    );
  }
);

interface FocusOnSubtreeProps {
  selectedNode: ProfileHeaderProps['selectedNode'];
  onFocusOnSubtree: ProfileHeaderProps['onFocusOnSubtree'];
  showMode: ReturnType<typeof useSizeMode>;
}
function FocusOnSubtree({
  onFocusOnSubtree,
  selectedNode,
  showMode,
}: FocusOnSubtreeProps) {
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
  const onClick = f.isNothing
    ? () => {}
    : () => {
        onFocusOnSubtree(f.value.i, f.value.j);
      };

  return (
    <Button
      disabled={!selectedNode.isJust}
      onClick={onClick}
      icon={faCompressAlt}
    >
      {text}
    </Button>
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
    <>
      <Button
        id="reset"
        data-testid="reset-view"
        disabled={!isFlamegraphDirty}
        onClick={reset}
        icon={faUndo}
      >
        {text}
      </Button>
    </>
  );
}

function FitMode({ fitMode, updateFitMode, showMode }) {
  let texts = {
    header: '',
    head: '',
    tail: '',
  };

  switch (showMode) {
    case 'small': {
      texts = {
        header: 'Fit',
        head: 'Head',
        tail: 'Tail',
      };
      break;
    }
    case 'large': {
      texts = {
        header: 'Prefer to Fit',
        head: 'Head first',
        tail: 'Tail first',
      };
      break;
    }

    default:
      throw new Error('Wrong mode');
  }

  return (
    <select
      aria-label="fit-mode"
      className={styles['fit-mode-select']}
      value={fitMode}
      onChange={(event) => updateFitMode(event.target.value)}
    >
      <option disabled>{texts.header}</option>
      <option value={FitModes.HEAD}>{texts.head}</option>
      <option value={FitModes.TAIL}>{texts.tail}</option>
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
      <option value="icicle">Flame</option>
    </select>
  );

  const kindByState = (name: string) => {
    if (view === name) {
      return 'primary';
    }
    return 'default';
  };

  const Buttons = (
    <>
      <Button
        grouped
        kind={kindByState('table')}
        icon={faTable}
        onClick={() => updateView('table')}
      >
        Table
      </Button>
      <Button
        grouped
        kind={kindByState('both')}
        icon={faColumns}
        onClick={() => updateView('both')}
      >
        Both
      </Button>
      <Button
        grouped
        kind={kindByState('icicle')}
        icon={faIcicles}
        onClick={() => updateView('icicle')}
      >
        Flamegraph
      </Button>
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

export default Toolbar;
