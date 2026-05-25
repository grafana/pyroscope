import './Icon.css';

export type IconType =
  | 'angle-down'
  | 'angle-left'
  | 'angle-right'
  | 'angle-up'
  | 'check'
  | 'logo'
  | 'moon'
  | 'play'
  | 'plus'
  | 'refresh'
  | 'sandwich'
  | 'search'
  | 'sun'
  | 'times';

export function Icon({ name, size = 16 }: { name: IconType; size?: number }) {
  return (
    <span
      className="icon"
      style={{
        width: size,
        height: size,
        maskImage: `url(icons/${name}.svg)`,
        WebkitMaskImage: `url(icons/${name}.svg)`,
      }}
    />
  );
}
