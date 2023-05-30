import React from 'react';
import { faFolder } from '@fortawesome/free-solid-svg-icons/faFolder';
import { faFolderOpen } from '@fortawesome/free-solid-svg-icons/faFolderOpen';
import { faAngleRight } from '@fortawesome/free-solid-svg-icons/faAngleRight';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from 'classnames/bind';
import styles from '@pyroscope/webapp/javascript/components/AppSelector/SelectButton.module.scss';

const cx = classNames.bind(styles);

export const MENU_ITEM_ROLE = 'menuitem';

interface SelectButtonProps {
  name: string;
  isSelected: boolean;
  onClick: () => void;
  icon: 'folder' | 'pyroscope';
}

function PyroscopeLogo({ className }: { className: string }) {
  return (
    <svg
      className={className}
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 183.34 249.26"
    >
      <g>
        <g>
          <path d="M91.73,0C111.2,28.89,116.2,60,108,93.53l.47.07,26.22-32.69c1,1.38,1.92,2.65,2.83,3.93C144.65,75,151.8,85.23,159.05,95.36c2.25,3.15,4.94,6,7.2,9.11A90,90,0,0,1,182,142.06a91.18,91.18,0,0,1,1.14,21.87,90.07,90.07,0,0,1-14,42.68,91.33,91.33,0,0,1-15.84,18.8,7.06,7.06,0,0,0-.47-.91q-9-12.25-18.07-24.49a1,1,0,0,1,.06-1.59,64.9,64.9,0,0,0,12.31-19.12,56.68,56.68,0,0,0,3.73-17.66A56,56,0,0,0,147.21,137c-8.27-20.69-23.34-33.59-45.31-37.63A59,59,0,0,0,35.1,140a57.72,57.72,0,0,0-2.72,19.39,59.23,59.23,0,0,0,45,55.79c15,3.68,29.46,2.11,43-5.66a5.93,5.93,0,0,1,.56-.29c.11-.05.24-.08.52-.18l19.31,26.19c-3.34,1.77-6.51,3.58-9.78,5.18a87.67,87.67,0,0,1-26.55,8,92.12,92.12,0,0,1-22.14.37,89.24,89.24,0,0,1-34.75-10.89A91.79,91.79,0,0,1,13.31,205,90.85,90.85,0,0,1,1.23,172.75a88.49,88.49,0,0,1-1-21.25c1.25-18,7.51-34.14,18-48.77Q50.82,57.46,83.13,12C86,8.09,88.76,4.15,91.73,0Z" />
          <path d="M134.92,157.67a43.24,43.24,0,1,1-43.07-43.22A43.16,43.16,0,0,1,134.92,157.67Z" />
        </g>
      </g>
    </svg>
  );
}

const Icon = ({
  isSelected,
  icon,
}: {
  icon: SelectButtonProps['icon'];
  isSelected: boolean;
}) => {
  if (icon === 'folder') {
    return (
      <FontAwesomeIcon
        className={styles.itemIcon}
        icon={isSelected ? faFolderOpen : faFolder}
      />
    );
  }

  return <PyroscopeLogo className={styles.pyroscopeLogo} />;
};

export const SelectButton = ({
  icon,
  name,
  isSelected,
  onClick,
}: SelectButtonProps) => {
  return (
    <button
      role={MENU_ITEM_ROLE}
      type="button"
      onClick={onClick}
      className={cx(styles.button, isSelected && styles.isSelected)}
      title={name}
    >
      <div>
        <Icon icon={icon} isSelected={isSelected} />
        <div className={styles.appName}>{name}</div>
      </div>
      <div>
        {icon === 'folder' ? (
          <FontAwesomeIcon className={styles.chevron} icon={faAngleRight} />
        ) : null}
      </div>
    </button>
  );
};
