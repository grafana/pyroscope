import React from 'react';
import { faCog } from '@fortawesome/free-solid-svg-icons/faCog';
import { faUser } from '@fortawesome/free-solid-svg-icons/faUser';
import { MenuButton, MenuProps, MenuHeader } from '@szhsin/react-menu';
import Dropdown, { MenuItem as DropdownMenuItem } from '@webapp/ui/Dropdown';
import flattenChildren from 'react-flatten-children';
import Icon from '@webapp/ui/Icon';
import { MenuItem } from '@webapp/ui/Sidebar';
import {
  selectIsMultiTenant,
  selectTenantID,
  actions,
} from '@webapp/redux/reducers/tenant';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import styles from '@phlare/components/SidebarTenant.module.css';
import cx from 'classnames';

export interface DropdownProps {
  children: JSX.Element[] | JSX.Element;
  offsetX: MenuProps['offsetX'];
  offsetY: MenuProps['offsetY'];
  direction: MenuProps['direction'];
  label: string;
  className: string;
  menuButton: JSX.Element;
}

function FlatDropdown({
  children,
  offsetX,
  offsetY,
  direction,
  label,
  className,
  menuButton,
}: DropdownProps) {
  return (
    <Dropdown
      offsetX={offsetX}
      offsetY={offsetY}
      direction={direction}
      label={label}
      className={className}
      menuButton={menuButton}
    >
      {flattenChildren(children) as unknown as JSX.Element}
    </Dropdown>
  );
}

export function SidebarTenant() {
  const isMultiTenant = useAppSelector(selectIsMultiTenant);
  const orgID = useAppSelector(selectTenantID);
  const dispatch = useAppDispatch();

  if (!isMultiTenant) {
    return <></>;
  }

  return (
    <>
      <FlatDropdown
        offsetX={10}
        offsetY={5}
        direction="top"
        label=""
        className={styles.dropdown}
        menuButton={
          <MenuButton className={styles.accountDropdown}>
            <MenuItem icon={<Icon icon={faUser} />}>Tenant</MenuItem>
          </MenuButton>
        }
      >
        <MenuHeader>Current Tenant</MenuHeader>
        <DropdownMenuItem
          className={styles.menuItemDisabled}
          onClick={() => {
            void dispatch(actions.setWantsToChange());
          }}
        >
          <div className={styles.menuItemWithButton}>
            <span className={cx(styles.menuItemWithButtonTitle, styles.orgID)}>
              Tenant ID: {orgID}
            </span>
            <Icon icon={faCog} />
          </div>
        </DropdownMenuItem>
      </FlatDropdown>
    </>
  );
}
