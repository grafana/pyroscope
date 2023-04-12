import React, {
  ReactNode,
  RefObject,
  useState,
  useRef,
  useLayoutEffect,
  isValidElement,
  memo,
  useCallback,
} from 'react';
import { faProjectDiagram } from '@fortawesome/free-solid-svg-icons/faProjectDiagram';
import { Maybe } from 'true-myth';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import useResizeObserver from '@react-hook/resize-observer';
// until ui is moved to its own package this should do it
// eslint-disable-next-line import/no-extraneous-dependencies
import { Tooltip } from '@pyroscope/webapp/javascript/ui/Tooltip';
import { Button as GButton } from '@grafana/ui';
import { FitModes, HeadMode, TailMode } from './fitMode/fitMode';
import SharedQueryInput from './SharedQueryInput';
import type { ViewTypes } from './FlameGraph/FlameGraphComponent/viewTypes';
import type { FlamegraphRendererProps } from './FlameGraph/FlameGraphRenderer';
import {
  TableIcon,
  TablePlusFlamegraphIcon,
  FlamegraphIcon,
  SandwichIcon,
} from './Icons';

import styles from './Toolbar.module.scss';

const QUERY_INPUT_WIDTH = 175;
const MORE_BUTTON_WIDTH = 16;

const calculateCollapsedItems = (
  clientWidth: number,
  itemsCollapsed: boolean,
  itemsW: number[]
) => {
  const availableToolbarItemsWidth = itemsCollapsed
    ? clientWidth - QUERY_INPUT_WIDTH - MORE_BUTTON_WIDTH - 5
    : clientWidth - QUERY_INPUT_WIDTH - 5;

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

const BUTTON_WIDTH = 32;
const BUTTON_MARGIN = 4;

/**
 * Computes if a "More" button should be shown and if so, which items should be hidden. It hides a sections of the
 * toolbar not individual buttons.
 * @param target
 * @param toolbarSections
 */
const useMoreButton = (
  target: RefObject<HTMLDivElement>,
  toolbarSections: { el: ReactNode; buttons: number }[]
) => {
  const toolbarItemsWidth = toolbarSections.reduce(
    (acc, v) => [...acc, v.buttons * (BUTTON_WIDTH + BUTTON_MARGIN)],
    [] as number[]
  );

  const [isMenuOpen, setMenuOpen] = useState(true);
  const [collapsedItemsNumber, setCollapsedItemsNumber] = useState(0);

  const handleSizeChange = useCallback(
    (wrapper: Element) => {
      const { width } = wrapper.getBoundingClientRect();
      const collapsedItems = calculateCollapsedItems(
        width,
        collapsedItemsNumber > 0,
        toolbarItemsWidth
      );
      setCollapsedItemsNumber(collapsedItems);
    },
    [collapsedItemsNumber, toolbarItemsWidth]
  );

  useLayoutEffect(() => {
    if (target.current) {
      handleSizeChange(target.current);
    }
  }, [target, handleSizeChange]);

  useResizeObserver(target, (entry: ResizeObserverEntry) => {
    handleSizeChange(entry.target);
    setMenuOpen(false);
  });

  return {
    isMenuOpen,
    handleMoreClick: () => {
      setMenuOpen((v) => !v);
    },
    hiddenItems: toolbarSections
      .slice(0, collapsedItemsNumber)
      .map((i) => i.el),
    visibleItems: toolbarSections.slice(collapsedItemsNumber).map((i) => i.el),
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

    // Sections of a toolbar, sections have a divider between them and can consist of multiple buttons
    const filteredToolbarSections = [
      {
        el: <FitMode fitMode={fitMode} updateFitMode={updateFitMode} />,
        buttons: 2,
      },
      {
        el: (
          <>
            <ResetView isFlamegraphDirty={isFlamegraphDirty} reset={reset} />
            <FocusOnSubtree
              selectedNode={selectedNode}
              onFocusOnSubtree={onFocusOnSubtree}
            />
          </>
        ),
        buttons: 2,
      },
    ];

    if (enableChangingDisplay) {
      filteredToolbarSections.push({
        el: (
          <ViewSection
            flamegraphType={flamegraphType}
            view={view}
            updateView={updateView}
          />
        ),
        // sandwich view is hidden in diff view
        buttons: flamegraphType === 'single' ? 4 : 5,
      });
    }

    if (isValidElement(ExportData)) {
      filteredToolbarSections.push({
        el: ExportData,
        buttons: 1,
      });
    }

    // Check if we have enough space to display all the buttons. If not we will show a "More" button where we will
    // hide some buttons.
    const { isMenuOpen, handleMoreClick, hiddenItems, visibleItems } =
      useMoreButton(toolbarRef, filteredToolbarSections);

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
              <ToolbarButtons items={visibleItems} />
              {hiddenItems.length > 0 && (
                <Tooltip placement="top" title="More">
                  <GButton
                    onClick={handleMoreClick}
                    variant="secondary"
                    fill="outline"
                    icon="ellipsis-v"
                  />
                </Tooltip>
              )}
            </div>
          </div>
          {isMenuOpen && (
            <div className={styles.navbarCollapsedItems}>
              <ToolbarButtons items={hiddenItems} />
            </div>
          )}
        </div>
      </div>
    );
  }
);

function ToolbarButtons(props: { items: React.ReactNode[] }) {
  return (
    <>
      {props.items.map((el, i) => (
        // eslint-disable-next-line react/no-array-index-key
        <div key={i} className={styles.item}>
          {el}
          {i !== props.items.length - 1 && <div className={styles.divider} />}
        </div>
      ))}
    </>
  );
}

function FocusOnSubtree({
  onFocusOnSubtree,
  selectedNode,
}: {
  selectedNode: ProfileHeaderProps['selectedNode'];
  onFocusOnSubtree: ProfileHeaderProps['onFocusOnSubtree'];
}) {
  return (
    <Tooltip placement="top" title="Collapse nodes above">
      <div>
        <GButton
          disabled={!selectedNode.isJust}
          onClick={() => {
            if (selectedNode.isJust) {
              onFocusOnSubtree(selectedNode.value.i, selectedNode.value.j);
            }
          }}
          aria-label="Collapse nodes above"
          icon="sort-amount-up"
        />
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
        <GButton
          id="reset"
          disabled={!isFlamegraphDirty}
          onClick={reset}
          aria-label="Reset View"
          icon="history-alt"
          style={{ marginRight: BUTTON_MARGIN }}
        />
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
        <GButton
          onClick={() => updateFitMode(HeadMode)}
          variant="secondary"
          fill={isSelected(HeadMode) ? 'solid' : 'outline'}
          icon="horizontal-align-left"
          style={{ marginRight: BUTTON_MARGIN }}
        />
      </Tooltip>
      <Tooltip placement="top" title="Tail first">
        <GButton
          onClick={() => updateFitMode(TailMode)}
          variant="secondary"
          fill={isSelected(TailMode) ? 'solid' : 'outline'}
          icon="horizontal-align-right"
        />
      </Tooltip>
    </>
  );
}

type ViewOptionItem = {
  label: string;
  value: ViewTypes;
  Icon: (props: { fill?: string | undefined }) => JSX.Element;
};

const getViewOptions = (
  flamegraphType: ProfileHeaderProps['flamegraphType']
): Array<ViewOptionItem> => {
  let options: Array<ViewOptionItem> = [
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
  if (flamegraphType === 'single') {
    options = options.concat([
      { label: 'Sandwich', value: 'sandwich', Icon: SandwichIcon },
      {
        label: 'GraphViz',
        value: 'graphviz',
        Icon: () => <FontAwesomeIcon icon={faProjectDiagram} />,
      },
    ]);
  }
  return options;
};

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
          <GButton
            data-testid={value}
            onClick={() => updateView(value)}
            variant="secondary"
            fill={view === value ? 'solid' : 'outline'}
            style={{ marginRight: BUTTON_MARGIN }}
          >
            <div
              // Weird styling but this is to get same size as with built in icons which don't take any actual space
              // and the sizing is done by padding of the button. With this we basically get rid of the icons actual
              // size of 16px and the size of the button is just it's padding same as with the icon prop.
              style={{
                width: 16,
                marginLeft: -8,
                marginRight: -8,
              }}
            >
              <Icon />
            </div>
          </GButton>
        </Tooltip>
      ))}
    </div>
  );
}

export default Toolbar;
