import React, { useMemo, useState } from 'react';
import classNames from 'classnames/bind';
import OutsideClickHandler from 'react-outside-click-handler';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './Menu.module.scss';

const cx = classNames.bind(styles);

const getInnerText = (elem: HTMLElement | null | undefined) => {
  return elem?.innerText as string;
};

const selectionModes = [
  { value: '=', meaning: 'include' },
  { value: '!=', meaning: 'exclude' },
  { value: '=~', meaning: 'include_or' },
  { value: '~!', meaning: 'exclude_or' },
];

const SelectionModeRadio = ({
  prefix,
  value,
}: {
  prefix: string;
  value?: 'include' | 'exclude';
}) => {
  const incId = `${prefix}_include`;
  const excId = `${prefix}_exclude`;

  return (
    <div className={styles.radioGroup}>
      Tag selection mode
      <div className={styles.radioList}>
        {value ? (
          <>
            <input
              id={excId}
              name="selection_mode"
              type="radio"
              value="exclude"
              checked={value === 'exclude'}
              className={cx(styles.radio, 'exclude')}
              onChange={() => {}}
            />
            <label htmlFor={excId}>exclude</label>
            <input
              id={incId}
              name="selection_mode"
              type="radio"
              value="include"
              checked={value === 'include'}
              className={cx(styles.radio, 'include')}
              onChange={() => {}}
            />
            <label htmlFor={incId}>include</label>{' '}
          </>
        ) : (
          <div className={styles.selectionError}>Error!</div>
        )}
      </div>
    </div>
  );
};

const Menu = ({ query, parent }: { query: string; parent: Element }) => {
  const [visible, toggle] = useState(false);
  const attrValue = getInnerText(parent?.parentElement);
  const attrPunctuation = getInnerText(
    parent?.parentElement?.previousElementSibling as HTMLElement
  );
  const attrName = getInnerText(
    parent?.parentElement?.previousElementSibling
      ?.previousElementSibling as HTMLElement
  );

  const radioPrefix = visible
    ? `${attrName}${attrPunctuation}${attrValue}`
    : null;

  const selectionMode = useMemo(() => {
    return visible
      ? selectionModes.find((item) => item.value === attrPunctuation)
      : undefined;
  }, [visible, attrPunctuation]);

  const selectionModeRadioValue: 'include' | 'exclude' | undefined =
    useMemo(() => {
      if (!selectionMode) {
        return undefined;
      }

      return selectionMode.meaning === 'include' ||
        selectionMode.meaning === 'include_or'
        ? 'include'
        : 'exclude';
    }, [selectionMode]);

  return (
    <OutsideClickHandler onOutsideClick={() => toggle(false)}>
      <div
        role="none"
        onClick={() => toggle(!visible)}
        className={styles.menuInnerWrapper}
      >
        <div
          className={cx({ visible }, styles.menu)}
          aria-hidden="true"
          onClick={(e) => {
            console.log('query', query);

            e.stopPropagation();
          }}
        >
          {radioPrefix ? (
            <SelectionModeRadio
              prefix={radioPrefix}
              value={selectionModeRadioValue}
            />
          ) : null}
          <div className={styles.menuItem}>Menu Item I</div>
          <div className={styles.menuItem}>Menu Item II</div>
          <div className={styles.menuItem}>Menu Item III</div>
        </div>
      </div>
    </OutsideClickHandler>
  );
};

export default Menu;
