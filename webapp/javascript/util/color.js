import murmurhash3_32_gc from './murmur3';
import Color from 'color';

const colors = [
  Color.hsl(24,  69,  60),
  Color.hsl(34,  65,  65),
  Color.hsl(194, 52,  61),
  Color.hsl(163, 45,  55),
  Color.hsl(211, 48,  60),
  Color.hsl(246, 40,  65),
  Color.hsl(305, 63,  79),
  Color.hsl(47,  100, 73),

  Color.rgb(183, 219, 171),
  Color.rgb(244, 213, 152),
  Color.rgb(112, 219, 237),
  Color.rgb(249, 186, 143),
  Color.rgb(242, 145, 145),
  Color.rgb(130, 181, 216),
  Color.rgb(229, 168, 226),
  Color.rgb(174, 162, 224),
  Color.rgb(154, 196, 138),
  Color.rgb(242, 201, 109),
  Color.rgb(101, 197, 219),
  Color.rgb(249, 147, 78),
  Color.rgb(234, 100, 96),
  Color.rgb(81,  149, 206),
  Color.rgb(214, 131, 206),
  Color.rgb(128, 110, 183),
]

export const defaultColor = Color.rgb(148, 142, 142);
export const diffColorRed = Color.rgb(200, 0, 0);
export const diffColorGreen = Color.rgb(0, 170, 0);

export function colorBasedOnPackageName(name, a) {
  const hash = murmurhash3_32_gc(name);
  const colorIndex = hash % colors.length;
  const baseClr = colors[colorIndex];
  return baseClr.alpha(a);
}

// assume: left >= 0 && Math.abs(diff) <= left so diff / left is in [0...1]
// if left == 0 || Math.abs(diff) > left, we use the color of 100%
export function colorBasedOnDiff(diff, left, a) {
  const v = !left || Math.abs(diff) > left ? 1
    : 200 * Math.sqrt(Math.abs(diff / left));
  if (diff >= 0) return Color.rgb(200,  200-v,200-v).alpha(a);
  return Color.rgb(200-v, 200, 200-v).alpha(a);
}

export function colorGreyscale(v, a) {
  return Color.rgb(v, v, v).alpha(a);
}
