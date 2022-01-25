import React, { useEffect, useState } from 'react';
import { ComponentMeta } from '@storybook/react';
import DiffPaletteDropdown from '../webapp/javascript/components/FlameGraph/FlameGraphComponent/DiffLegendPaletteDropdown';

export default {
  title: 'DiffPaletteDropdown',
} as ComponentMeta<typeof DiffPaletteDropdown>;

export const DefaultDiffPaletteDropdown = () => {
  const [midWidth, setMidwith] = useState(400);

  useEffect(() => {
    const interval = setInterval(() => {
      setMidwith(midWidth === 400 ? 800 : 400);
    }, 2000);
    return () => clearInterval(interval);
  });

  return (
    <div>
      <div style={{ width: '300px', border: '1px solid red' }}>
        <DiffPaletteDropdown />
      </div>
      <div
        style={{
          width: '100%',
          border: '1px solid green',
          display: 'flex',
          alignItems: 'center',
          flexDirection: 'column',
        }}
      >
        <DiffPaletteDropdown />
      </div>
      <div style={{ width: `${midWidth}px`, border: '1px solid blue' }}>
        <DiffPaletteDropdown />
      </div>
    </div>
  );
};
