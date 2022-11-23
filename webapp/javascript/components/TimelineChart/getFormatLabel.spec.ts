import getFormatLabel from './getFormatLabel';

describe('getFormatLabel renders correct time format if:', () => {
  it('time distance is less than 12h - [HH:mm:ss]', () => {
    expect(
      getFormatLabel({
        date: 1662113437142.8572,
        timezone: 'utc',
        xaxis: { max: 1662114305000, min: 1662112505000 },
      })
    ).toEqual('10:10:37');
  });

  it('time distance is between 12h and 24h - [HH:mm]', () => {
    expect(
      getFormatLabel({
        date: 1662091559736.842,
        timezone: 'utc',
        xaxis: { max: 1662114865000, min: 1662071665000 },
      })
    ).toEqual('04:05');
  });

  it('time distance is greater than 12h - [MMM do HH:mm]', () => {
    expect(
      getFormatLabel({
        date: 1661611895545.1128,
        timezone: 'utc',
        xaxis: { max: 1662114865000, min: 1661510165000 },
      })
    ).toEqual('Aug 27th 14:51');
  });

  it('incorrect input - [???]', () => {
    expect(
      getFormatLabel({
        date: 123123123123123333,
        timezone: 'utc',
        xaxis: { min: -1, max: -2 },
      })
    ).toEqual('???');
  });
});
