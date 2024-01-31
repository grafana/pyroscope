import React from 'react';

type CodeProps = {
  lines: Line[];
};

export type Line = {
  line: string;
  number: number;
  cum: number;
  flat: number;
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
  const totalSelf = lines.reduce((acc, { flat }) => acc + flat, 0);
  const totalTotal = lines.reduce((acc, { cum }) => acc + cum, 0);
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
      {lines.map(({ line, number, cum: cum, flat: flat }) => (
        <div
          key={line + number + cum + flat}
          style={{
            color: flat === 0 ? 'gray' : '#ccccdc',
          }}
        >
          <span> {number}</span>
          <span>
            {fmtNumber(flat).slice(number.toString().length + 1)}
            {fmtNumber(cum)}
            {`          ${line}`}
          </span>
        </div>
      ))}
    </pre>
  );
};
export default Code;
