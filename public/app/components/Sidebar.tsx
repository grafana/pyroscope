import React from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faChevronLeft } from '@fortawesome/free-solid-svg-icons/faChevronLeft';

import SidebarUI, {
  MenuItem,
  SidebarHeader,
  SidebarFooter,
  SidebarContent,
  Menu,
} from '@webapp/ui/Sidebar';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  selectSidebarCollapsed,
  collapseSidebar,
  uncollapseSidebar,
  recalculateSidebar,
} from '@webapp/redux/reducers/ui';
import { useLocation, NavLink } from 'react-router-dom';
import Icon from '@webapp/ui/Icon';
import clsx from 'clsx';
import { useWindowWidth } from '@react-hook/window-size';
import { isRouteActive, ROUTES } from '@phlare/pages/routes';
import Logo from '@phlare/static/logo.svg';
import styles from '@webapp/components/Sidebar.module.css';
import { SidebarTenant } from '@phlare/components/SidebarTenant';

export function Sidebar() {
  const collapsed = useAppSelector(selectSidebarCollapsed);
  const dispatch = useAppDispatch();

  const { search, pathname } = useLocation();
  const windowWidth = useWindowWidth();

  React.useLayoutEffect(() => {
    dispatch(recalculateSidebar());
  }, [windowWidth]);

  const toggleCollapse = () => {
    const action = collapsed ? uncollapseSidebar : collapseSidebar;
    dispatch(action());
  };

  return (
    <SidebarUI collapsed={collapsed}>
      <SidebarHeader>
        <div className={styles.logo}>
          <Logo className={styles.logoImg} />
          <span
            className={clsx(styles.logoText, {
              [styles.logoTextCollapsed]: collapsed,
            })}
          >
            Pyroscope
          </span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <Menu iconShape="square" popperArrow>
          <MenuItem
            active={isRouteActive(pathname, ROUTES.SINGLE_VIEW)}
            icon={<Icon icon={faWindowMaximize} />}
          >
            Single View
            <NavLink
              activeClassName="active-route"
              to={{ pathname: ROUTES.SINGLE_VIEW, search }}
              exact
            />
          </MenuItem>
          <MenuItem
            active={isRouteActive(pathname, ROUTES.COMPARISON_VIEW)}
            icon={<Icon icon={faColumns} />}
          >
            Comparison View
            <NavLink to={{ pathname: ROUTES.COMPARISON_VIEW, search }} exact />
          </MenuItem>
          <MenuItem
            active={isRouteActive(pathname, ROUTES.COMPARISON_DIFF_VIEW)}
            icon={<Icon icon={faChartBar} />}
          >
            Diff View
            <NavLink
              to={{ pathname: ROUTES.COMPARISON_DIFF_VIEW, search }}
              exact
            />
          </MenuItem>
          <SidebarTenant />
        </Menu>
      </SidebarContent>
      <SidebarFooter>
        <Menu iconShape="square">
          <MenuItem
            data-testid="collapse-sidebar"
            className={clsx(
              styles.collapseIcon,
              collapsed ? styles.collapsedIconCollapsed : ''
            )}
            onClick={toggleCollapse}
            icon={<Icon icon={faChevronLeft} />}
          >
            Collapse Sidebar
          </MenuItem>
        </Menu>
      </SidebarFooter>
    </SidebarUI>
  );
}
