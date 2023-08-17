/* eslint-disable no-unused-expressions, import/no-extraneous-dependencies */
import React, { useCallback, useRef } from 'react';
import clsx from 'clsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faRedo } from '@fortawesome/free-solid-svg-icons/faRedo';
import { faCopy } from '@fortawesome/free-solid-svg-icons/faCopy';
import { faHighlighter } from '@fortawesome/free-solid-svg-icons/faHighlighter';
import { faCompressAlt } from '@fortawesome/free-solid-svg-icons/faCompressAlt';
import { MenuItem } from '@phlare/ui/Menu';
import useResizeObserver from '@react-hook/resize-observer';
import { Maybe } from 'true-myth';
import debounce from 'lodash.debounce';
import { Flamebearer } from '@phlare/legacy/models';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';
import Highlight from './Highlight';
import ContextMenuHighlight from './ContextMenuHighlight';
import FlamegraphTooltip from '../../Tooltip/FlamegraphTooltip';
import ContextMenu from './ContextMenu';
import LogoLink from './LogoLink';
import { SandwichIcon, HeadFirstIcon, TailFirstIcon } from '../../Icons';
import { PX_PER_LEVEL } from './constants';
import Header from './Header';
import { FlamegraphPalette } from './colorPalette';
import type { ViewTypes } from './viewTypes';
import { FitModes, HeadMode, TailMode } from '../../fitMode/fitMode';
import indexStyles from './styles.module.scss';

interface FlamegraphProps {
  flamebearer: Flamebearer;
  focusedNode: ConstructorParameters<typeof Flamegraph>[2];
  fitMode: ConstructorParameters<typeof Flamegraph>[3];
  updateFitMode: (f: FitModes) => void;
  highlightQuery: ConstructorParameters<typeof Flamegraph>[4];
  zoom: ConstructorParameters<typeof Flamegraph>[5];
  showCredit: boolean;
  selectedItem: Maybe<string>;

  onZoom: (bar: Maybe<{ i: number; j: number }>) => void;
  onFocusOnNode: (i: number, j: number) => void;
  setActiveItem: (item: { name: string }) => void;
  updateView?: (v: ViewTypes) => void;

  onReset: () => void;
  isDirty: () => boolean;

  ['data-testid']?: string;
  palette: FlamegraphPalette;
  setPalette: (p: FlamegraphPalette) => void;
  toolbarVisible?: boolean;
  headerVisible?: boolean;
  disableClick?: boolean;
  showSingleLevel?: boolean;
}

export default function FlameGraphComponent(props: FlamegraphProps) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);
  const flamegraph = useRef<Flamegraph>();

  const [rightClickedNode, setRightClickedNode] = React.useState<
    Maybe<{ top: number; left: number; width: number }>
  >(Maybe.nothing());

  const {
    flamebearer,
    focusedNode,
    fitMode,
    updateFitMode,
    highlightQuery,
    zoom,
    toolbarVisible,
    headerVisible = true,
    disableClick = false,
    showSingleLevel = false,
    showCredit,
    setActiveItem,
    selectedItem,
    updateView,
  } = props;

  const { onZoom, onReset, isDirty, onFocusOnNode } = props;
  const { 'data-testid': dataTestId } = props;
  const { palette, setPalette } = props;

  const canvasEl = canvasRef?.current;
  const currentFlamegraph = flamegraph?.current;

  const renderCanvas = useCallback(() => {
    canvasEl?.setAttribute('data-state', 'rendering');
    currentFlamegraph?.render();
    canvasEl?.setAttribute('data-state', 'rendered');
  }, [canvasEl, currentFlamegraph]);

  // debounce rendering canvas
  // used for situations like resizing
  // triggered by eg collapsing the sidebar
  const debouncedRenderCanvas = useCallback(() => {
    debounce(() => {
      renderCanvas();
    }, 50);
  }, [renderCanvas]);

  // rerender whenever the canvas size changes
  // eg window resize, or simply changing the view
  // to display the flamegraph isolated from the table
  useResizeObserver(canvasRef, () => {
    if (flamegraph) {
      debouncedRenderCanvas();
    }
  });

  const onClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const opt = getFlamegraph().xyToBar(
      e.nativeEvent.offsetX,
      e.nativeEvent.offsetY
    );

    opt.match({
      // clicked on an invalid node
      Nothing: () => {},
      Just: (bar) => {
        zoom.match({
          // there's no existing zoom
          // so just zoom on the clicked node
          Nothing: () => {
            onZoom(opt);
          },

          // it's already zoomed
          Just: (z) => {
            // TODO there mya be stale props here...
            // we are clicking on the same node that's zoomed
            if (bar.i === z.i && bar.j === z.j) {
              // undo that zoom
              onZoom(Maybe.nothing());
            } else {
              onZoom(opt);
            }
          },
        });
      },
    });
  };

  const xyToHighlightData = (x: number, y: number) => {
    const opt = getFlamegraph().xyToBar(x, y);

    return opt.map((bar) => {
      return {
        left: getCanvas().offsetLeft + bar.x,
        top: getCanvas().offsetTop + bar.y,
        width: bar.width,
      };
    });
  };

  const xyToTooltipData = (x: number, y: number) => {
    return getFlamegraph().xyToBar(x, y);
  };

  const onContextMenuClose = () => {
    setRightClickedNode(Maybe.nothing());
  };

  const onContextMenuOpen = (x: number, y: number) => {
    setRightClickedNode(xyToHighlightData(x, y));
  };

  // Context Menu stuff
  const xyToContextMenuItems = useCallback(
    (x: number, y: number) => {
      const dirty = isDirty();
      const bar = getFlamegraph().xyToBar(x, y);
      const barName = bar.isJust ? bar.value.name : '';

      const CollapseItem = () => {
        const hoveredOnValidNode = bar.mapOrElse(
          () => false,
          () => true
        );

        const onClick = bar.mapOrElse(
          () => () => {},
          (f) => onFocusOnNode.bind(null, f.i, f.j)
        );

        return (
          <MenuItem
            key="focus"
            disabled={!hoveredOnValidNode}
            onClick={onClick}
          >
            <FontAwesomeIcon icon={faCompressAlt} />
            Collapse nodes above
          </MenuItem>
        );
      };

      const CopyItem = () => {
        const onClick = () => {
          if (!navigator.clipboard) {
            return;
          }

          navigator.clipboard.writeText(barName);
        };

        return (
          <MenuItem key="copy" onClick={onClick}>
            <FontAwesomeIcon icon={faCopy} />
            Copy function name
          </MenuItem>
        );
      };

      const HighlightSimilarNodesItem = () => {
        const onClick = () => {
          setActiveItem({ name: barName });
        };

        const actionName =
          selectedItem.isJust && selectedItem.value === barName
            ? 'Clear highlight'
            : 'Highlight similar nodes';

        return (
          <MenuItem key="highlight-similar-nodes" onClick={onClick}>
            <FontAwesomeIcon icon={faHighlighter} />
            {actionName}
          </MenuItem>
        );
      };

      const OpenInSandwichViewItem = () => {
        if (!updateView) {
          return null;
        }

        const handleClick = () => {
          if (updateView) {
            updateView('sandwich');
            setActiveItem({ name: barName });
          }
        };

        return (
          <MenuItem
            key="open-in-sandwich-view"
            className={indexStyles.sandwichItem}
            onClick={handleClick}
          >
            <SandwichIcon fill="black" />
            Open in sandwich view
          </MenuItem>
        );
      };

      const FitModeItem = () => {
        const isHeadFirst = fitMode === HeadMode;

        const handleClick = () => {
          const newValues = isHeadFirst ? TailMode : HeadMode;
          updateFitMode(newValues);
        };

        return (
          <MenuItem
            className={indexStyles.fitModeItem}
            key="fit-mode"
            onClick={handleClick}
          >
            {isHeadFirst ? <TailFirstIcon /> : <HeadFirstIcon />}
            Show text {isHeadFirst ? 'tail first' : 'head first'}
          </MenuItem>
        );
      };

      return [
        <MenuItem key="reset" disabled={!dirty} onClick={onReset}>
          <FontAwesomeIcon icon={faRedo} />
          Reset View
        </MenuItem>,
        CollapseItem(),
        CopyItem(),
        HighlightSimilarNodesItem(),
        OpenInSandwichViewItem(),
        FitModeItem(),
      ].filter(Boolean) as JSX.Element[];
    },
    [
      selectedItem,
      fitMode,
      isDirty,
      onFocusOnNode,
      onReset,
      setActiveItem,
      updateFitMode,
      updateView,
    ]
  );

  React.useEffect(() => {
    if (canvasEl) {
      const f = new Flamegraph(
        flamebearer,
        canvasEl,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom,
        palette
      );
      flamegraph.current = f;
    }
    renderCanvas();
  }, [
    canvasEl,
    palette,
    flamebearer,
    focusedNode,
    fitMode,
    highlightQuery,
    zoom,
    renderCanvas,
  ]);

  const dataUnavailable =
    !flamebearer || (flamebearer && flamebearer.names.length <= 1);

  const getCanvas = () => {
    if (!canvasRef.current) {
      throw new Error('Missing canvas');
    }
    return canvasRef.current;
  };

  const getFlamegraph = () => {
    if (!flamegraph.current) {
      throw new Error('Missing canvas');
    }
    return flamegraph.current;
  };

  return (
    <div
      data-testid="flamegraph-view"
      className={clsx(indexStyles.flamegraphPane, {
        'vertical-orientation': flamebearer.format === 'double',
      })}
    >
      {headerVisible && (
        <Header
          format={flamebearer.format}
          units={flamebearer.units}
          palette={palette}
          setPalette={setPalette}
          toolbarVisible={toolbarVisible}
        />
      )}
      <div
        data-testid={dataTestId}
        style={{
          opacity: dataUnavailable && !showSingleLevel ? 0 : 1,
        }}
      >
        <canvas
          height="0"
          data-testid="flamegraph-canvas"
          data-highlightquery={highlightQuery}
          className={clsx('flamegraph-canvas', styles.canvas)}
          ref={canvasRef}
          onClick={!disableClick ? onClick : undefined}
        />
      </div>
      {showCredit ? <LogoLink /> : ''}
      {flamegraph && canvasRef && (
        <Highlight
          barHeight={PX_PER_LEVEL}
          canvasRef={canvasRef}
          zoom={zoom}
          xyToHighlightData={xyToHighlightData}
        />
      )}
      {flamegraph && (
        <ContextMenuHighlight
          barHeight={PX_PER_LEVEL}
          node={rightClickedNode}
        />
      )}
      {flamegraph && (
        <FlamegraphTooltip
          format={flamebearer.format}
          canvasRef={canvasRef}
          xyToData={xyToTooltipData as ShamefulAny}
          numTicks={flamebearer.numTicks}
          sampleRate={flamebearer.sampleRate}
          leftTicks={
            flamebearer.format === 'double' ? flamebearer.leftTicks : 0
          }
          rightTicks={
            flamebearer.format === 'double' ? flamebearer.rightTicks : 0
          }
          units={flamebearer.units}
          palette={palette}
        />
      )}

      {!disableClick && flamegraph && canvasRef && (
        <ContextMenu
          canvasRef={canvasRef}
          xyToMenuItems={xyToContextMenuItems}
          onClose={onContextMenuClose}
          onOpen={onContextMenuOpen}
        />
      )}
    </div>
  );
}
