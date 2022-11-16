import React from 'react';

export function HeatmapSelectionIcon() {
  return (
    <svg
      id="selection_included_svg"
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 512 512"
    >
      <rect style={{ fill: '#492d13' }} y=".69" width="512" height="510.62" />
      <rect
        style={{ fill: '#bac437' }}
        x="1.41"
        y=".12"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#76844b' }}
        x="307.46"
        y="102.26"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#76844b' }}
        x="103.43"
        y="204.4"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#9eb941' }}
        x="205.44"
        y="305.97"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#fde823' }}
        x="307.46"
        y="-.41"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#9eb941' }}
        x="1.41"
        y="408.12"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#760d24' }}
        x="307.46"
        y="408.12"
        width="102.4"
        height="102.12"
      />
      <rect
        style={{ fill: '#fde823' }}
        x="409.47"
        y="408.12"
        width="102.4"
        height="102.12"
      />
      <path
        style={{ fill: '#fead19' }}
        d="M513.5,513.5H-1.5V-1.5H513.5V513.5ZM4.5,507.5H507.5V4.5H4.5V507.5Z"
      />
    </svg>
  );
}

export function HeatmapNoSelectionIcon() {
  return (
    <svg
      id="selection_excluded_svg"
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 512 512"
    >
      <rect style={{ fill: '#161616' }} y=".19" width="512" height="511.62" />
      <rect
        style={{ fill: '#c1d84f' }}
        x="1.28"
        y=".13"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#468d89' }}
        x="307.33"
        y="102.47"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#468d89' }}
        x="103.3"
        y="204.82"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#7ac46e' }}
        x="205.31"
        y="306.58"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#f9e555' }}
        x="307.33"
        y="-.4"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#7ac46e' }}
        x="1.28"
        y="408.93"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#422970' }}
        x="307.33"
        y="408.93"
        width="102.4"
        height="102.32"
      />
      <rect
        style={{ fill: '#f9e555' }}
        x="409.35"
        y="408.93"
        width="102.4"
        height="102.32"
      />
      <path
        style={{ fill: '#a5a2a0' }}
        d="M513.5,513.5H-1.5V-1.5H513.5V513.5ZM4.5,507.5H507.5V4.5H4.5V507.5Z"
      />
    </svg>
  );
}
