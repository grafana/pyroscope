import React, { useCallback, useRef } from 'react';
import clsx from 'clsx';
import { MenuItem } from '@szhsin/react-menu';
import useResizeObserver from '@react-hook/resize-observer';
import { Maybe } from 'true-myth';
import debounce from 'lodash.debounce';
import { Flamebearer } from '@pyroscope/models';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';
import Highlight from './Highlight';
import ContextMenuHighlight from './ContextMenuHighlight';
import Tooltip from './Tooltip';
import ContextMenu from './ContextMenu';
import { PX_PER_LEVEL } from './constants';
import Header from './Header';
import { FlamegraphPalette } from './colorPalette';
import indexStyles from './styles.module.scss';

interface FlamegraphProps {
  flamebearer: Flamebearer;
  focusedNode: ConstructorParameters<typeof Flamegraph>[2];
  fitMode: ConstructorParameters<typeof Flamegraph>[3];
  highlightQuery: ConstructorParameters<typeof Flamegraph>[4];
  zoom: ConstructorParameters<typeof Flamegraph>[5];

  onZoom: (bar: Maybe<{ i: number; j: number }>) => void;
  onFocusOnNode: (i: number, j: number) => void;

  onReset: () => void;
  isDirty: () => boolean;

  ExportData?: React.ComponentProps<typeof Header>['ExportData'];

  ['data-testid']?: string;
  palette: FlamegraphPalette;
  setPalette: (p: FlamegraphPalette) => void;
}

export default function FlameGraphComponent(props: FlamegraphProps) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);
  const flamegraph = useRef<Flamegraph>();

  const [rightClickedNode, setRightClickedNode] = React.useState<
    Maybe<{ top: number; left: number; width: number }>
  >(Maybe.nothing());

  const { flamebearer, focusedNode, fitMode, highlightQuery, zoom } = props;

  const { onZoom, onReset, isDirty, onFocusOnNode } = props;
  const { ExportData } = props;
  const { 'data-testid': dataTestId } = props;
  const { palette, setPalette } = props;

  // debounce rendering canvas
  // used for situations like resizing
  // triggered by eg collapsing the sidebar
  const debouncedRenderCanvas = useCallback(
    debounce(() => {
      renderCanvas();
    }, 50),
    []
  );

  // rerender whenever the canvas size changes
  // eg window resize, or simply changing the view
  // to display the flamegraph isolated from the table
  useResizeObserver(canvasRef, (e) => {
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

      const FocusItem = () => {
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
            Focus on this subtree
          </MenuItem>
        );
      };

      return [
        <MenuItem key="reset" disabled={!dirty} onClick={onReset}>
          Reset View
        </MenuItem>,
        FocusItem(),
      ];
    },
    [flamegraph]
  );

  const constructCanvas = () => {
    if (canvasRef.current) {
      const f = new Flamegraph(
        flamebearer,
        canvasRef.current,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom,
        palette
      );

      flamegraph.current = f;
    }
  };

  React.useEffect(() => {
    constructCanvas();
    renderCanvas();
  }, [palette]);

  React.useEffect(() => {
    constructCanvas();
    renderCanvas();
  }, [
    canvasRef.current,
    flamebearer,
    focusedNode,
    fitMode,
    highlightQuery,
    zoom,
  ]);

  const renderCanvas = () => {
    // eslint-disable-next-line no-unused-expressions
    flamegraph?.current?.render();
  };

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
      <Header
        format={flamebearer.format}
        units={flamebearer.units}
        ExportData={ExportData}
        palette={palette}
        setPalette={setPalette}
      />

      {dataUnavailable ? (
        <div className={styles.error}>
          <span>
            No profiling data available for this application / time range.
          </span>
        </div>
      ) : null}
      <div
        data-testid={dataTestId}
        style={{
          opacity: dataUnavailable ? 0 : 1,
        }}
      >
        <canvas
          height="0"
          data-testid="flamegraph-canvas"
          data-highlightquery={highlightQuery}
          className={clsx('flamegraph-canvas', styles.canvas)}
          ref={canvasRef}
          onClick={onClick}
        />
      </div>
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
        <Tooltip
          format={flamebearer.format}
          canvasRef={canvasRef}
          xyToData={xyToTooltipData as any /* TODO */}
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

      {flamegraph && canvasRef && (
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
