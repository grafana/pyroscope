import React, { useState } from 'react';
import classNames from 'classnames/bind';
import OutsideClickHandler from 'react-outside-click-handler';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './Menu.module.scss';

const cx = classNames.bind(styles);

const getInnerText = (elem: HTMLElement | null | undefined) => {
  return elem?.innerText as string;
};

const SelectionModeRadio = ({ prefix }: { prefix: string }) => {
  const incId = `${prefix}_include`;
  const excId = `${prefix}_exclude`;

  return (
    <div className={styles.radioGroup}>
      Tag selection mode
      <div className={styles.radioList}>
        <input
          id={excId}
          name="selection_mode"
          type="radio"
          value="exclude"
          className={cx(styles.radio, 'exclude')}
        />
        <label htmlFor={excId}>exclude</label>
        <input
          id={incId}
          name="selection_mode"
          type="radio"
          value="include"
          className={cx(styles.radio, 'include')}
        />
        <label htmlFor={incId}>include</label>
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

  return (
    <OutsideClickHandler onOutsideClick={() => toggle(false)}>
      <div
        role="none"
        onClick={() => toggle(!visible)}
        className={styles.menuInnerWrapper}
      >
        <div
          className={cx({ menu: true, visible })}
          aria-hidden="true"
          onClick={(e) => {
            console.log('query', query);

            e.stopPropagation();
          }}
        >
          {radioPrefix ? <SelectionModeRadio prefix={radioPrefix} /> : null}
          <div className={styles.menuItem}>Menu Item I</div>
          <div className={styles.menuItem}>Menu Item II</div>
          <div className={styles.menuItem}>Menu Item III</div>
        </div>
      </div>
    </OutsideClickHandler>
  );
};

export default Menu;
