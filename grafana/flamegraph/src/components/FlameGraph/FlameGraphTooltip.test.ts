import { getTooltipData } from './FlameGraphTooltip';

describe('should get tooltip data correctly', () => {
  it('for bytes', () => {
    const tooltipData = getTooltipData('memory:alloc_space:bytes:space:bytes', 'total', 8_624_078_250, 8_624_078_250);
    expect(tooltipData).toEqual({
      name: 'total',
      percentTitle: '% of total RAM',
      percentValue: 100,
      unitTitle: 'RAM',
      unitValue: '8.03 GB',
      samples: '8,624,078,250',
    });
  });

  it('for objects', () => {
    const tooltipData = getTooltipData('memory:alloc_objects:count:space:bytes', 'total', 8_624_078_250, 8_624_078_250);
    expect(tooltipData).toEqual({
      name: 'total',
      percentTitle: '% of total objects',
      percentValue: 100,
      unitTitle: 'Allocated objects',
      unitValue: '8.62 G',
      samples: '8,624,078,250',
    });
  });

  it('for nanoseconds', () => {
    const tooltipData = getTooltipData(
      'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      'total',
      8_624_078_250,
      8_624_078_250
    );
    expect(tooltipData).toEqual({
      name: 'total',
      percentTitle: '% of total time',
      percentValue: 100,
      unitTitle: 'Time',
      unitValue: '8.62 seconds',
      samples: '8,624,078,250',
    });
  });
});
