import React from 'react';
import {
  faFolder,
  faFolderOpen,
  faAngleRight,
  faFire,
} from '@fortawesome/free-solid-svg-icons';
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
        <FontAwesomeIcon icon={getIcon(isFolder, isSelected)} />
        {name}
      </div>
      <div>{isFolder ? <FontAwesomeIcon icon={faAngleRight} /> : null}</div>
    </button>
  );
};

export default SelectButton;
