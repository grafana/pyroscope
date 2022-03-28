import React from 'react';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faIcicles } from '@fortawesome/free-solid-svg-icons/faIcicles';
import { faListUl } from '@fortawesome/free-solid-svg-icons/faListUl';
import { faTable } from '@fortawesome/free-solid-svg-icons/faTable';
import { faUndo } from '@fortawesome/free-solid-svg-icons/faUndo';
import { faCompressAlt } from '@fortawesome/free-solid-svg-icons/faCompressAlt';
import { DebounceInput } from 'react-debounce-input';
import { Maybe } from 'true-myth';
import useResizeObserver from '@react-hook/resize-observer';
// until ui is moved to its own package this should do it
// eslint-disable-next-line import/no-extraneous-dependencies
import Button from '@webapp/ui/Button';
import { FitModes, HeadMode, TailMode } from './fitMode/fitMode';
import { ViewTypes } from './FlameGraph/FlameGraphComponent/viewTypes';

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
  view: ViewTypes;
  disableChangingDisplay?: boolean;
  flamegraphType: 'single' | 'double';
  viewDiff: 'diff' | 'total' | 'self';
  handleSearchChange: (s: string) => void;
  highlightQuery: string;
  renderLogo: boolean;

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
}

// TODO: move this to assets pipeline. for now just embedding it here because this is less likely to break
function svgLogo() {
  return (
    <svg
      width="40px"
      height="40px"
      viewBox="0 0 1024 1024"
      version="1.1"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <radialGradient
          cx="49.4236252%"
          cy="92.6627823%"
          fx="49.4236252%"
          fy="92.6627823%"
          r="195.066755%"
          gradientTransform="translate(0.494236,0.926628),scale(1.000000,0.735610),rotate(-90.000000),translate(-0.494236,-0.926628)"
          id="radialGradient-1"
        >
          <stop stopColor="#FFB90C" offset="0%" />
          <stop stopColor="#F9243A" offset="38.390924%" />
          <stop stopColor="#F9243A" offset="50.5405%" />
          <stop stopColor="#B51424" offset="73.98091%" />
          <stop stopColor="#B51424" offset="100%" />
        </radialGradient>
      </defs>
      <g
        id="Artboard"
        stroke="none"
        strokeWidth="1"
        fill="none"
        fillRule="evenodd"
      >
        <g
          id="fire-part"
          transform="translate(148.516736, 0.000000)"
          fillRule="nonzero"
        >
          <g
            id="whole-thing"
            transform="translate(363.983264, 495.000000) scale(-1, 1) rotate(-180.000000) translate(-363.983264, -495.000000) translate(0.483264, 0.000000)"
          >
            <g
              id="g70"
              transform="translate(-0.000091, 0.685815)"
              fill="url(#radialGradient-1)"
            >
              <path
                d="M65.3646667,571.739321 L65.4492471,571.698868 C19.5139147,505.999969 -5.32464048,424.477859 1.04305801,336.877516 L1.04305801,336.877516 C14.0321963,158.179446 159.192462,13.7596653 338.059844,1.5917266 L338.059844,1.5917266 C419.418369,-3.93888015 495.500283,17.3823334 558.456522,57.4611191 L558.456522,57.4611191 L481.301947,162.097965 C437.516468,136.521928 399.367671,129.590556 363.486536,130.155994 L363.486536,130.155994 C234.497143,130.155994 129.556988,235.032238 129.556988,363.946998 L129.556988,363.946998 C129.556988,492.865683 234.497143,597.738003 363.486536,597.738003 L363.486536,597.738003 C492.483783,597.738003 597.427864,492.865683 597.427864,363.946998 L597.427864,363.946998 C597.41276,304.634864 581.39383,255.677522 530.630465,199.668053 L607.770843,95.1329436 C680.936847,161.576603 726.932594,257.364176 726.932594,363.946998 L726.932594,363.946998 C726.932594,458.031616 691.13483,543.75602 632.416071,608.271816 L632.416071,608.271816 L632.416071,608.275741 L533.597728,748.122808 L428.601388,617.203806 L434.703262,646.563419 C459.453008,765.59222 433.664131,889.543925 363.49439,988.853335 L363.49439,988.853335 L65.3646667,571.723019 L65.3646667,571.739321 Z"
                id="path84"
              />
            </g>
            <g id="blue" transform="translate(191.447039, 191.331780)">
              <g id="g88" transform="translate(-0.000063, 0.685930)">
                <g
                  id="g94"
                  transform="translate(0.177296, 0.699054)"
                  fill="#3EC1D3"
                >
                  <path
                    d="M171.862466,343.697728 C77.0961324,343.697728 -0.00497405932,266.647602 -0.00497405932,171.934957 C-0.00497405932,77.2182874 77.0961324,0.168162396 171.862466,0.168162396 C266.632828,0.168162396 343.741988,77.2182874 343.741988,171.934957 C343.741988,266.647602 266.632828,343.697728 171.862466,343.697728"
                    id="path96"
                  />
                </g>
                <g
                  id="g98"
                  transform="translate(29.362379, 172.629585)"
                  fill="#FFFFFF"
                >
                  <path
                    d="M22.8397982,0 L0.671669409,0 C0.671669409,78.2496309 64.380874,141.920035 142.678189,141.920035 L142.678189,119.765407 C76.6007327,119.765407 22.8397982,66.0372141 22.8397982,0"
                    id="path100"
                  />
                </g>
              </g>
            </g>
          </g>
        </g>
      </g>
    </svg>
  );
}

function logo() {
  return (
    <a
      className={styles.headerLogo}
      href="https://github.com/pyroscope-io/pyroscope/"
      target="_blank"
      rel="noreferrer"
    >
      {[svgLogo(), <span>Pyroscope</span>]}
    </a>
  );
}

const Toolbar = React.memo(
  ({
    view,
    viewDiff,
    handleSearchChange,
    highlightQuery,
    renderLogo,
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
  }: ProfileHeaderProps) => {
    const toolbarRef = React.useRef<HTMLDivElement>(null);
    const showMode = useSizeMode(toolbarRef);

    return (
      <div role="toolbar" ref={toolbarRef} data-mode={showMode}>
        <div className={styles.navbar}>
          {renderLogo ? logo() : ''}
          <HighlightSearch
            showMode={showMode}
            onHighlightChange={handleSearchChange}
            highlightQuery={highlightQuery}
          />
          {flamegraphType === 'double' && (
            <DiffView
              showMode={showMode}
              viewDiff={viewDiff}
              updateViewDiff={updateViewDiff}
            />
          )}
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
          {!disableChangingDisplay && (
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

  const onClick = selectedNode.mapOr(
    () => {},
    (f) => {
      return () => onFocusOnSubtree(f.i, f.j);
    }
  );

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

function HighlightSearch({
  onHighlightChange,
  showMode,
  highlightQuery,
}: {
  showMode: ReturnType<typeof useSizeMode>;
  onHighlightChange: ProfileHeaderProps['handleSearchChange'];
  highlightQuery: ProfileHeaderProps['highlightQuery'];
}) {
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
      value={highlightQuery}
    />
  );
}

function ResetView({
  isFlamegraphDirty,
  reset,
  showMode,
}: {
  showMode: ReturnType<typeof useSizeMode>;
  isFlamegraphDirty: ProfileHeaderProps['isFlamegraphDirty'];
  reset: ProfileHeaderProps['reset'];
}) {
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
      onChange={(event) => updateFitMode(event.target.value as typeof fitMode)}
    >
      <option disabled>{texts.header}</option>
      <option value={HeadMode}>{texts.head}</option>
      <option value={TailMode}>{texts.tail}</option>
    </select>
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

  const Select = (
    <select
      name="viewDiff"
      aria-label="view-diff"
      value={viewDiff}
      onChange={(e) => {
        updateViewDiff(e.target.value as typeof viewDiff);
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

function ViewSection({
  view,
  updateView,
  showMode,
}: {
  showMode: ReturnType<typeof useSizeMode>;
  updateView: ProfileHeaderProps['updateView'];
  view: ProfileHeaderProps['view'];
}) {
  const Select = (
    <select
      aria-label="view"
      name="view"
      value={view}
      onChange={(e) => {
        updateView(e.target.value as typeof view);
      }}
    >
      <option value="table">Table</option>
      <option value="both">Both</option>
      <option value="flamegraph">Flame</option>
    </select>
  );

  const kindByState = (name: ViewTypes) => {
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
        kind={kindByState('flamegraph')}
        icon={faIcicles}
        onClick={() => updateView('flamegraph')}
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
