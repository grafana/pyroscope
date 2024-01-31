import { getValueFormat, ValueFormatter } from '@grafana/data';
import React from 'react';

export type CodeProps = {
  unit: string;
  lines: Line[];
};

export type Line = {
  line: string;
  number: number;
  cum: number;
  flat: number;
};

const Code = ({ lines, unit }: CodeProps) => {
  const totalSelf = lines.reduce((acc, { flat }) => acc + flat, 0);
  const totalTotal = lines.reduce((acc, { cum }) => acc + cum, 0);
  const fmt = formatter(unit);

  function formatValue(n: number): string {
    if (n === 0) {
      return '           .';
    }
    let fmted = fmt(n);
    const txt = fmted.text + fmted.suffix;
    if (`${txt}`.length <= 13) {
      return `${txt}`.padStart(12, ' ');
    }
    return txt;
  }

  return (
    <pre
      style={{
        fontFamily: 'monospace',
        fontSize: '12px',
        overflowX: 'auto',
        whiteSpace: 'pre',
      }}
    >
      <div>
        <span>
          Total:
          {formatValue(totalSelf).slice(4)}
          {formatValue(totalTotal)}
          {` `}(flat, cum)
        </span>
      </div>
      {lines.map(({ line, number, cum: cum, flat: flat }) => (
        <div
          key={line + number + cum + flat}
          style={{
            color: flat + cum === 0 ? 'gray' : '#ccccdc',
          }}
        >
          <span> {number}</span>
          <span>
            {formatValue(flat).slice(number.toString().length + 1)}
            {formatValue(cum)}
            {`          ${line}`}
          </span>
        </div>
      ))}
    </pre>
  );
};
export default Code;

function formatter(unit: string): ValueFormatter {
  switch (unit) {
    case 'nanoseconds':
      return getValueFormat('ns');
    case 'microseconds':
      return getValueFormat('µs');
    case 'milliseconds':
      return getValueFormat('ms');
    case 'seconds':
      return getValueFormat('s');
    case 'count':
      return (n: number) => ({ text: `${n}`, suffix: '' });
    default:
      return getValueFormat(unit);
  }
}
