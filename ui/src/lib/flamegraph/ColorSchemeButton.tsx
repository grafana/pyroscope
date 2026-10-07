import { byPackageGradient, byValueGradient } from './FlameGraph/colors';
import { Popover, PopoverItem } from './Popover';
import { ColorScheme } from './types';
import './ColorSchemeButton.css';

type ColorSchemeButtonProps = {
  value: ColorScheme;
  onChange: (colorScheme: ColorScheme) => void;
};

export function ColorSchemeButton(props: ColorSchemeButtonProps) {
  const gradient =
    props.value === ColorScheme.PackageBased
      ? byPackageGradient
      : byValueGradient;

  return (
    <Popover
      trigger={({ toggle }) => (
        <button
          type="button"
          className="fg-cs-button"
          onClick={toggle}
          aria-label="Change color scheme"
          title="Change color scheme"
        >
          <span className="fg-cs-dot" style={{ background: gradient }} />
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
