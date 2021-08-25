import { fitToCanvasRect, FitModes } from '../javascript/util/fitMode';


describe('fitToCanvasRect', () => {
  describe('HEAD', () => {
    it("always returns fullText", () => {
      const mode = FitModes.HEAD;
      const charSize = 1;
      const rectWidth = 10;
      const fullText = "full_long_text";
      const shortText = "short_text";
      const shortTextWidth = shortText.length * charSize;

      expect(fitToCanvasRect({
        mode,
        charSize,
        rectWidth,
        fullText,
        shortText,
        shortTextWidth
      })).toMatchObject({
        mode,
        text: fullText,
        marginLeft: 3,
      });
    });
  });

  describe("TAIL", () => {
    it("returns full text if it CAN fit", () => {
      const mode = FitModes.TAIL;
      const charSize = 1;
      const rectWidth = 99;
      const fullText = "full_long_text";
      const shortText = "short_text";
      const shortTextWidth = shortText.length * charSize;

      expect(fitToCanvasRect({
        mode,
        charSize,
        rectWidth,
        fullText,
        shortText,
        shortTextWidth
      })).toMatchObject({
        mode,
        text: fullText,
        marginLeft: 3,
      });
    });

    it("returns short text with if it CAN fit short text", () => {
      const mode = FitModes.TAIL;
      const charSize = 1;
      const rectWidth = 10;
      const fullText = "full_long_text";
      const shortText = "short_text";
      const shortTextWidth = shortText.length * charSize;

      expect(fitToCanvasRect({
        mode,
        charSize,
        rectWidth,
        fullText,
        shortText,
        shortTextWidth
      })).toMatchObject({
        mode,
        text: shortText,
        marginLeft: 3,
      });
    });

    it("returns short text with negative margin BOTH short and long CAN NOT fit", () => {
      const mode = FitModes.TAIL;
      const charSize = 1;
      const rectWidth = 10;
      const fullText = "full_long_text"; // 14
      const shortText = "short_text_"; // 11
      const shortTextWidth = shortText.length * charSize;

      expect(fitToCanvasRect({
        mode,
        charSize,
        rectWidth,
        fullText,
        shortText,
        shortTextWidth
      })).toMatchObject({
        mode,
        text: shortText,
        marginLeft: -4,
      });
    });
  });
});
