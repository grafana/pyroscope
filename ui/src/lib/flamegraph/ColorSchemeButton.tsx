import { css, cx } from '@emotion/css';

import { byPackageGradient, byValueGradient } from './FlameGraph/colors';
import { Popover, PopoverItem } from './Popover';
import { ColorScheme } from './types';

type ColorSchemeButtonProps = {
  value: ColorScheme;
  onChange: (colorScheme: ColorScheme) => void;
};

export function ColorSchemeButton(props: ColorSchemeButtonProps) {
  const colorDotStyle =
    props.value === ColorScheme.PackageBased ? styles.colorDotByPackage : styles.colorDotByValue;

  return (
    <Popover
      trigger={({ toggle }) => (
        <button
          type="button"
          className={styles.button}
          onClick={toggle}
          aria-label="Change color scheme"
          title="Change color scheme"
        >
          <span className={cx(styles.colorDot, colorDotStyle)} />
        </button>
      )}
      overlay={({ close }) => (
        <>
          <PopoverItem
            label="By package name"
            active={props.value === ColorScheme.PackageBased}
            onClick={() => {
              props.onChange(ColorScheme.PackageBased);
              close();
            }}
          />
          <PopoverItem
            label="By value"
            active={props.value === ColorScheme.ValueBased}
            onClick={() => {
              props.onChange(ColorScheme.ValueBased);
              close();
            }}
          />
        </>
      )}
    />
  );
}

const styles = {
  button: css({
    label: 'colorSchemeButton',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    height: 28,
    padding: '0 8px',
    marginRight: 8,
    background: 'transparent',
    color: 'var(--text-primary)',
    border: '1px solid var(--color-secondary-border)',
    borderRadius: 'var(--radius-md)',
    cursor: 'pointer',
    '&:hover': { background: 'var(--action-hover)' },
  }),
  colorDot: css({
    label: 'colorDot',
    display: 'inline-block',
    width: 10,
    height: 10,
    borderRadius: '50%',
  }),
  colorDotByValue: css({
    label: 'colorDotByValue',
    background: byValueGradient,
  }),
  colorDotByPackage: css({
    label: 'colorDotByPackage',
    background: byPackageGradient,
  }),
};
