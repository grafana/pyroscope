import React from 'react';
import { numberWithCommas } from './format';

export default function Tooltip(props) {
  const { format, canvasRef, xyToData, isWithinBounds, formatter } = props;
  const [body, setBody] = React.useState('');
  const [title, setTitle] = React.useState('');
  const [style, setStyle] = React.useState();
  const tooltipEl = React.useRef(null);

  const onMouseMove = (e) => {
    if (!isWithinBounds(e.offsetX, e.offsetY)) {
      onMouseOut();
      return;
    }

    //    const { left, right } = xyToData(e.offsetX, e.offsetX);
    const data = xyToData(format, e.offsetX, e.offsetY);

    // where to position
    const left = Math.min(
      e.clientX + 12,
      window.innerWidth - tooltipEl.current.clientWidth - 20
    );
    const top = e.clientY + 20;

    setTitle(data.title);
    setStyle({
      top,
      left,
      opacity: 1,
    });

    // format is either single, double or something else
    switch (format) {
      case 'single': {
        const d = formatSingle(
          formatter,
          data.percent,
          data.numBarTicks,
          data.sampleRate
        );
        console.log({ d });

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
    // watch for mouse events on the bar
    canvasRef.current.addEventListener('mousemove', onMouseMove);
    canvasRef.current.addEventListener('mouseout', onMouseOut);

    return () => {
      canvasRef.current.removeEventListener('mousemove', onMouseMove);
      canvasRef.current.removeEventListener('mouseout', onMouseOut);
    };
  });

  return (
    <div className="flamegraph-tooltip" style={style} ref={tooltipEl}>
      <div className="flamegraph-tooltip-name">{title}</div>
      <div>{body}</div>
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
