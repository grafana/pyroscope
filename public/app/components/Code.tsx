import React from 'react';

type CodeProps = {
  lines: Line[];
};

export type Line = {
  line: string;
  number: number;
  total: number;
  self: number;
};

function fmtNumber(n: number): string {
  if (n === 0) {
    return '           .';
  }
  if (`${n}`.length <= 13) {
    return `${n}`.padStart(12, ' ');
  }
  return n.toString();
}

const Code = ({ lines }: CodeProps) => {
  const totalSelf = lines.reduce((acc, { self }) => acc + self, 0);
  const totalTotal = lines.reduce((acc, { total }) => acc + total, 0);
  return (
    <pre
      style={{
        fontFamily: 'monospace',
        fontSize: '12px',
      }}
    >
      <div>
        <span>
          Total:
          {fmtNumber(totalSelf).slice(4)}
          {fmtNumber(totalTotal)}
          {` `}(flat, cum)
        </span>
      </div>
      {lines.map(({ line, number, total, self }) => (
        <div
          key={line + number + total + self}
          style={{
            color: self === 0 ? 'gray' : '#ccccdc',
          }}
        >
          <span> {number}</span>
          <span>
            {fmtNumber(self).slice(number.toString().length + 1)}
            {fmtNumber(total)}
            {`          ${line}`}
          </span>
        </div>
      ))}
    </pre>
  );
};
export default Code;
