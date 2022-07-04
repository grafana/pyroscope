import React from 'react';
import { faFolder } from '@fortawesome/free-solid-svg-icons/faFolder';
import { faFolderOpen } from '@fortawesome/free-solid-svg-icons/faFolderOpen';
import { faAngleRight } from '@fortawesome/free-solid-svg-icons/faAngleRight';
import { faFire } from '@fortawesome/free-solid-svg-icons/faFire';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from 'classnames/bind';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './SelectButton.module.scss';

const cx = classNames.bind(styles);

interface SelectButtonProps {
  name: string;
  fullList: string[];
  isSelected: boolean;
  onClick: () => void;
}

const getIcon = (isFolder: boolean, isSelected: boolean) => {
  if (isFolder) {
    return isSelected ? faFolderOpen : faFolder;
  }
  return faFire;
};

const SelectButton = ({
  name,
  fullList,
  isSelected,
  onClick,
}: SelectButtonProps) => {
  const isFolder = fullList.indexOf(name) === -1;

  return (
    <button
      role="menuitem"
      type="button"
      onClick={onClick}
      className={cx({ button: true, isSelected })}
    >
      <div>
        <FontAwesomeIcon
          className={styles.itemIcon}
          icon={getIcon(isFolder, isSelected)}
        />
        <div className={styles.appName}>{name}</div>
      </div>
      <div>{isFolder ? <FontAwesomeIcon icon={faAngleRight} /> : null}</div>
    </button>
  );
};

export default SelectButton;
