import React from 'react';
import { numberWithCommas, getFormatter } from './format';

export default function Tooltip(props) {
  const { format, canvasRef, xyToData, isWithinBounds } = props;
  const [body, setBody] = React.useState('');
  const [title, setTitle] = React.useState('');
  const [style, setStyle] = React.useState();
  const tooltipEl = React.useRef(null);

  const { numTicks, sampleRate, units } = props;
  // TODO cache this to not have to instantiate all the time?
  const formatter = getFormatter(numTicks, sampleRate, units);

  const onMouseMove = (e) => {
    if (!isWithinBounds(e.offsetX, e.offsetY)) {
      onMouseOut();
      return;
    }

    const data = xyToData(format, e.offsetX, e.offsetY);

    // where to position
    const left = Math.min(
      e.clientX + 12,
      window.innerWidth - tooltipEl.current.clientWidth - 20
    );
    const top = e.clientY + 20;

    setTitle(data.title);
    setStyle({ top, left, opacity: 1 });

    // format is either single, double or something else
    switch (format) {
      case 'single': {
        const d = formatSingle(
          formatter,
          data.percent,
          data.numBarTicks,
          props.sampleRate
        );

        setBody(d.tooltipText);
        break;
      }

      default:
        throw new Error(`Unsupported format ${format}`);
    }
  };

  const onMouseOut = () => {
    // Set visibility
    //    console.log("mouse out");
    setStyle({
      opacity: 0,
    });
  };

  React.useEffect(() => {
    // use closure to "cache" the current canvas reference
    // so that the clean up points to a valid canvas
    // (otherwise it would be null)
    const canvasEl = canvasRef.current;
    if (!canvasEl) {
      return {};
    }

    // watch for mouse events on the bar
    canvasEl.addEventListener('mousemove', onMouseMove);
    canvasEl.addEventListener('mouseout', onMouseOut);

    return () => {
      canvasEl.removeEventListener('mousemove', onMouseMove);
      canvasEl.removeEventListener('mouseout', onMouseOut);
    };
  }, [canvasRef.current]);

  return (
    <div className="flamegraph-tooltip" style={style} ref={tooltipEl}>
      <div
        data-testid="flamegraph-tooltip-title"
        className="flamegraph-tooltip-name"
      >
        {title}
      </div>
      <div data-testid="flamegraph-tooltip-body">{body}</div>
    </div>
  );
}

function formatSingle(formatter, percent, numBarTicks, sampleRate) {
  const tooltipText = `${percent}, ${numberWithCommas(
    numBarTicks
  )} samples, ${formatter.format(numBarTicks, sampleRate)}`;

  return {
    tooltipText,
  };
}

function formatDouble() {}
