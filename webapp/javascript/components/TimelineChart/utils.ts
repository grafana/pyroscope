/* eslint-disable no-nested-ternary */

export function clamp(min: number, value: number, max: number) {
  return value < min ? min : value > max ? max : value;
}
