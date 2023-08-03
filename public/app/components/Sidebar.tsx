import React from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons/faWindowMaximize';
import { faSearch } from '@fortawesome/free-solid-svg-icons/faSearch';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faChevronLeft } from '@fortawesome/free-solid-svg-icons/faChevronLeft';

import SidebarUI, {
  MenuItem,
  SidebarHeader,
  SidebarFooter,
  SidebarContent,
  Menu,
} from '@phlare/ui/Sidebar';
import { useAppSelector, useAppDispatch } from '@phlare/redux/hooks';
import {
  selectSidebarCollapsed,
  collapseSidebar,
  uncollapseSidebar,
  recalculateSidebar,
} from '@phlare/redux/reducers/ui';
import { useLocation, NavLink } from 'react-router-dom';
import Icon from '@phlare/ui/Icon';
import clsx from 'clsx';
import { useWindowWidth } from '@react-hook/window-size';
import { isRouteActive, ROUTES } from '@phlare/pages/routes';
import Logo from '@phlare/static/logo.svg';
import styles from './Sidebar.module.css';
import { SidebarTenant } from '@phlare/components/SidebarTenant';

export function Sidebar() {
  const collapsed = useAppSelector(selectSidebarCollapsed);
  const dispatch = useAppDispatch();

  const { search, pathname } = useLocation();
  const windowWidth = useWindowWidth();

  React.useLayoutEffect(() => {
    dispatch(recalculateSidebar());
  }, [dispatch, windowWidth]);

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
            data-testid="sidebar-explore-page"
            active={isRouteActive(pathname, ROUTES.EXPLORE_VIEW)}
            icon={<Icon icon={faSearch} />}
          >
            Tag Explorer
            <NavLink
              activeClassName="active-route"
              to={{ pathname: ROUTES.EXPLORE_VIEW, search }}
              exact
            />
          </MenuItem>
          <MenuItem
            data-testid="sidebar-continuous-single"
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
            data-testid="sidebar-continuous-comparison"
            active={isRouteActive(pathname, ROUTES.COMPARISON_VIEW)}
            icon={<Icon icon={faColumns} />}
          >
            Comparison View
            <NavLink to={{ pathname: ROUTES.COMPARISON_VIEW, search }} exact />
          </MenuItem>
          <MenuItem
            data-testid="sidebar-continuous-diff"
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
