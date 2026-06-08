import './Icon.css';

export type IconType =
  | 'align-left'
  | 'align-right'
  | 'angle-double-down'
  | 'angle-double-up'
  | 'angle-down'
  | 'angle-left'
  | 'angle-right'
  | 'angle-up'
  | 'check'
  | 'copy'
  | 'exclamation-circle'
  | 'eye'
  | 'history-alt'
  | 'logo'
  | 'moon'
  | 'play'
  | 'plus'
  | 'refresh'
  | 'sandwich'
  | 'search'
  | 'sun'
  | 'times';

export function Icon({
  name,
  size = 16,
  className,
}: {
  name: IconType;
  size?: number;
  className?: string;
}) {
  return (
    <span
      className={className ? `icon ${className}` : 'icon'}
      style={{
        width: size,
        height: size,
        maskImage: `url(icons/${name}.svg)`,
        WebkitMaskImage: `url(icons/${name}.svg)`,
      }}
    />
  );
}
