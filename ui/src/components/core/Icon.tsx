export type IconType =
  | 'angle-down'
  | 'angle-left'
  | 'angle-right'
  | 'check'
  | 'logo'
  | 'moon'
  | 'play'
  | 'plus'
  | 'refresh'
  | 'sun'
  | 'times';

export function Icon({ name, size = 16 }: { name: IconType; size?: number }) {
  return (
    <span
      style={{
        display: 'inline-block',
        width: size,
        height: size,
        flexShrink: 0,
        backgroundColor: 'currentColor',
        maskImage: `url(/icons/${name}.svg)`,
        maskSize: 'contain',
        maskRepeat: 'no-repeat',
        maskPosition: 'center',
        WebkitMaskImage: `url(/icons/${name}.svg)`,
        WebkitMaskSize: 'contain',
        WebkitMaskRepeat: 'no-repeat',
        WebkitMaskPosition: 'center',
      }}
    />
  );
}
