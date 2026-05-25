import { css, cx } from '@emotion/css';

import { type GrafanaTheme2 } from '@grafana/data';
import { Button, Dropdown, Menu, useStyles2 } from '@grafana/ui';

import { byPackageGradient, byValueGradient } from './FlameGraph/colors';
import { ColorScheme } from './types';

type ColorSchemeButtonProps = {
  value: ColorScheme;
  onChange: (colorScheme: ColorScheme) => void;
};

export function ColorSchemeButton(props: ColorSchemeButtonProps) {
  const styles = useStyles2(getStyles);
  const menu = (
    <Menu>
      <Menu.Item label="By package name" onClick={() => props.onChange(ColorScheme.PackageBased)} />
      <Menu.Item label="By value" onClick={() => props.onChange(ColorScheme.ValueBased)} />
    </Menu>
  );

  // Show a bit different gradient as a way to indicate selected value
  const colorDotStyle =
    {
      [ColorScheme.ValueBased]: styles.colorDotByValue,
      [ColorScheme.PackageBased]: styles.colorDotByPackage,
    }[props.value] || styles.colorDotByValue;

  return (
    <Dropdown overlay={menu}>
      <Button
        variant={'secondary'}
        fill={'outline'}
        size={'sm'}
        tooltip={'Change color scheme'}
        onClick={() => {}}
        className={styles.buttonSpacing}
        aria-label={'Change color scheme'}
      >
        <span className={cx(styles.colorDot, colorDotStyle)} />
      </Button>
    </Dropdown>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  buttonSpacing: css({
    label: 'buttonSpacing',
    marginRight: theme.spacing(1),
  }),
  colorDot: css({
    label: 'colorDot',
    display: 'inline-block',
    width: '10px',
    height: '10px',
    borderRadius: theme.shape.radius.circle,
  }),
  colorDotByValue: css({
    label: 'colorDotByValue',
    background: byValueGradient,
  }),
  colorDotByPackage: css({
    label: 'colorDotByPackage',
    background: byPackageGradient,
  }),
});
