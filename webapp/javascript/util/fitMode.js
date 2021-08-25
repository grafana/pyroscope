export const FitModes = {
  TAIL: "TAIL",
  HEAD: "HEAD",
}

const margin = 3;
export function fitToCanvasRect({ mode, charSize, rectWidth, fullText, shortText }){
  switch (mode) {
    case FitModes.TAIL:
      // Case 1:
      // content fits rectangle width
      // | rectangle |
      // | text |
      if (charSize * fullText.length <= rectWidth) { // assume it's a monospaced font
        return {
          mode,
          text: fullText,
          marginLeft: margin,
        }
      }

      // assume it's a monospaced font
      // if not the case, use
      // ctx.measureText(shortName).width
      const shortTextWidth = charSize * shortText.length;

      // Case 2:
      // short text fits rectangle width
      // | rectangle |
      // | long_text_text |
      // | shorttext |
      if (shortTextWidth <= rectWidth) { // assume it's a monospaced font
        return {
          mode,
          text: shortText,
          marginLeft: margin,
        }
      }

      // Case 3:
      // short text is bigger than rectangle width
      // add a negative margin left
      // so that the 'tail' of the string is visible
      //     | rectangle |
      // | my_short_text |
      return {
        mode,
        text: shortText,
        marginLeft: -((shortTextWidth - rectWidth) + margin),
      }

    // Case 3:
    // Normal
    case FitModes.HEAD:
    default:
      return {
        mode,
        text: fullText,
        marginLeft: margin,
      }
  }
}


