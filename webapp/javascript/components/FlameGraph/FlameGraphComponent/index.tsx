import React, { useCallback, useRef } from 'react';
import { Flamebearer } from '@models/flamebearer';
import clsx from 'clsx';
import { MenuItem } from '@szhsin/react-menu';
import useResizeObserver from '@react-hook/resize-observer';
import { Maybe } from '@utils/fp';
import debounce from 'lodash.debounce';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';
import Highlight from './Highlight';
import ContextMenuHighlight from './ContextMenuHighlight';
import Tooltip from './Tooltip';
import ContextMenu from './ContextMenu';
import { PX_PER_LEVEL } from './constants';
import Header from './Header';

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

  // the reason this is exposed as a parameter
  // is to not have to connect to the redux store from here
  ExportData: () => React.ReactElement;

  ['data-testid']?: string;
}

export default function FlameGraphComponent(props: FlamegraphProps) {
  const canvasRef = React.useRef<HTMLCanvasElement>();
  //  const [flamegraph, setFlamegraph] = React.useState<Flamegraph>();
  const flamegraph = useRef<Flamegraph>();

  const [rightClickedNode, setRightClickedNode] = React.useState<
    Maybe<{ top: number; left: number; width: number }>
  >(Maybe.nothing());

  const { flamebearer, focusedNode, fitMode, highlightQuery, zoom } = props;

  const { onZoom, onReset, isDirty, onFocusOnNode } = props;
  const { ExportData } = props;
  const { 'data-testid': dataTestId } = props;

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
    const opt = flamegraph.current.xyToBar(
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
    const opt = flamegraph.current.xyToBar(x, y);

    return opt.map((bar) => {
      return {
        left: canvasRef.current.offsetLeft + bar.x,
        top: canvasRef.current.offsetTop + bar.y,
        width: bar.width,
      };
    });
  };

  const xyToTooltipData = (x: number, y: number) => {
    return flamegraph.current.xyToBar(x, y);
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
      const bar = flamegraph.current.xyToBar(x, y);

      const FocusItem = () => {
        const hoveredOnValidNode = bar.mapOrElse(
          () => false,
          () => true
        );
        const onClick = bar.mapOrElse(
          () => {},
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

  React.useEffect(() => {
    if (canvasRef.current) {
      const f = new Flamegraph(
        flamebearer,
        canvasRef.current,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
      );

      flamegraph.current = f;
      renderCanvas();
    }
  }, [
    canvasRef.current,
    flamebearer,
    focusedNode,
    fitMode,
    highlightQuery,
    zoom,
  ]);

  const renderCanvas = () => {
    flamegraph.current.render();
  };

  const dataUnavailable =
    !flamebearer || (flamebearer && flamebearer.names.length <= 1);

  return (
    <div
      data-testid="flamegraph-view"
      className={clsx('flamegraph-pane', {
        'vertical-orientation': flamebearer.format === 'double',
      })}
    >
      <Header
        format={flamebearer.format}
        units={flamebearer.units}
        ExportData={ExportData}
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
          className={`flamegraph-canvas ${styles.hover}`}
          ref={canvasRef}
          onClick={onClick}
        />
      </div>
      {flamegraph && (
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
          leftTicks={flamebearer.format === 'double' && flamebearer.leftTicks}
          rightTicks={flamebearer.format === 'double' && flamebearer.rightTicks}
          units={flamebearer.units}
        />
      )}

      {flamegraph && (
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
