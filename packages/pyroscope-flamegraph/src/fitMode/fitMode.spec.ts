import { fitToCanvasRect } from './fitMode';

describe('fitToCanvasRect', () => {
  describe('HEAD', () => {
    it('always returns fullText', () => {
      const mode = 'HEAD';
      const charSize = 1;
      const rectWidth = 10;
      const fullText = 'full_long_text';
      const shortText = 'short_text';

      expect(
        fitToCanvasRect({
          mode,
          charSize,
          rectWidth,
          fullText,
          shortText,
        })
      ).toMatchObject({
        mode,
        text: fullText,
        marginLeft: 3,
      });
    });
  });

  describe('TAIL', () => {
    const mode = 'TAIL';
    it('returns full text if it CAN fit', () => {
      const charSize = 1;
      const rectWidth = 99;
      const fullText = 'full_long_text';
      const shortText = 'short_text';

      expect(
        fitToCanvasRect({
          mode,
          charSize,
          rectWidth,
          fullText,
          shortText,
        })
      ).toMatchObject({
        mode,
        text: fullText,
        marginLeft: 3,
      });
    });

    it('returns short text with if it CAN fit short text', () => {
      const charSize = 1;
      const rectWidth = 10;
      const fullText = 'full_long_text';
      const shortText = 'short_text';

      expect(
        fitToCanvasRect({
          mode,
          charSize,
          rectWidth,
          fullText,
          shortText,
        })
      ).toMatchObject({
        mode,
        text: shortText,
        marginLeft: 3,
      });
    });

    it('returns short text with negative margin BOTH short and long CAN NOT fit', () => {
      const charSize = 1;
      const rectWidth = 10;
      const fullText = 'full_long_text'; // 14
      const shortText = 'short_text_'; // 11

      expect(
        fitToCanvasRect({
          mode,
          charSize,
          rectWidth,
          fullText,
          shortText,
        })
      ).toMatchObject({
        mode,
        text: shortText,
        marginLeft: -4,
      });
    });
  });
});
