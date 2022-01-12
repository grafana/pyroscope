import React, { useEffect, useState } from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faSlack } from '@fortawesome/free-brands-svg-icons/faSlack';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import { faChevronLeft } from '@fortawesome/free-solid-svg-icons/faChevronLeft';
import { faSignOutAlt } from '@fortawesome/free-solid-svg-icons/faSignOutAlt';
import { faHandPointRight } from '@fortawesome/free-solid-svg-icons/faHandPointRight';
import { faSync } from '@fortawesome/free-solid-svg-icons/faSync';
import Sidebar, {
  MenuItem,
  SidebarHeader,
  SidebarFooter,
  SidebarContent,
  SubMenu,
  Menu,
} from '@ui/Sidebar';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import {
  selectSidebarCollapsed,
  collapseSidebar,
  uncollapseSidebar,
  recalculateSidebar,
} from '@pyroscope/redux/reducers/ui';
import { useLocation, NavLink } from 'react-router-dom';
import { isExperimentalAdhocUIEnabled } from '@utils/features';
import Icon from '@ui/Icon';
import { useWindowWidth } from '@react-hook/window-size';
import basename from '../util/baseurl';
import styles from './Sidebar.module.css';

// TODO: find a better way of doing this?
function signOut() {
  const form = document.createElement('form');

  form.method = 'POST';
  form.action = `${basename()}/logout`;

  document.body.appendChild(form);

  form.submit();
}

export default function Sidebar2() {
  const collapsed = useAppSelector(selectSidebarCollapsed);
  const dispatch = useAppDispatch();

  const { search, pathname } = useLocation();
  const windowWidth = useWindowWidth();

  // the component doesn't seem to support setting up an active item
  // so we must set it up manually
  // https://github.com/azouaoui-med/react-pro-sidebar/issues/84
  const isRouteActive = function (route: string) {
    return pathname === route;
  };

  React.useLayoutEffect(() => {
    dispatch(recalculateSidebar());
  }, [windowWidth]);

  // TODO
  // simplify this
  const isContinuousActive =
    isRouteActive('/') ||
    isRouteActive('/comparison') ||
    isRouteActive('/comparison-diff');
  const isAdhocActive =
    isRouteActive('/adhoc-single') ||
    isRouteActive('/adhoc-comparison') ||
    isRouteActive('/adhoc-comparison-diff');

  const adhoc = (
    <SubMenu
      title="Adhoc Profiling"
      icon={<Icon icon={faHandPointRight} />}
      active={isAdhocActive}
      defaultOpen={isAdhocActive}
      data-testid="sidebar-adhoc"
    >
      {collapsed && (
        <SidebarHeader className={styles.collapsedHeader}>
          Adhoc Profiling
        </SidebarHeader>
      )}
      <MenuItem
        data-testid="sidebar-adhoc-single"
        active={isRouteActive('/adhoc-single')}
        icon={<Icon icon={faWindowMaximize} />}
      >
        Single View
        <NavLink to={{ pathname: '/adhoc-single', search }} exact />
      </MenuItem>
      <MenuItem
        data-testid="sidebar-adhoc-comparison"
        active={isRouteActive('/adhoc-comparison')}
        icon={<Icon icon={faColumns} />}
      >
        Comparison View
        <NavLink to={{ pathname: '/adhoc-comparison', search }} exact />
      </MenuItem>
      <MenuItem
        data-testid="sidebar-adhoc-comparison-diff"
        active={isRouteActive('/adhoc-comparison-diff')}
        icon={<Icon icon={faChartBar} />}
      >
        Diff View
        <NavLink to={{ pathname: '/adhoc-comparison-diff', search }} exact />
      </MenuItem>
    </SubMenu>
  );

  const toggleCollapse = () => {
    const action = collapsed ? uncollapseSidebar : collapseSidebar;
    dispatch(action());
  };

  return (
    <Sidebar collapsed={collapsed}>
      <SidebarHeader>
        <div className={styles.logo}>
          <div className="logo-main" />
          <span className={`${collapsed ? styles.logoTextCollapsed : ''}`}>
            Pyroscope
          </span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <Menu iconShape="square" popperArrow>
          <SubMenu
            title="Continuous Profiling"
            icon={<Icon icon={faSync} />}
            active={isContinuousActive}
            defaultOpen={isContinuousActive}
            data-testid="sidebar-continuous"
          >
            {collapsed && (
              <SidebarHeader className={styles.collapsedHeader}>
                Continuous Profiling
              </SidebarHeader>
            )}
            <MenuItem
              data-testid="sidebar-continuous-single"
              active={isRouteActive('/')}
              icon={<Icon icon={faWindowMaximize} />}
            >
              Single View
              <NavLink
                activeClassName="active-route"
                data-testid="sidebar-root"
                to={{ pathname: '/', search }}
                exact
              />
            </MenuItem>
            <MenuItem
              data-testid="sidebar-continuous-comparison"
              active={isRouteActive('/comparison')}
              icon={<Icon icon={faColumns} />}
            >
              Comparison View
              <NavLink to={{ pathname: '/comparison', search }} exact />
            </MenuItem>
            <MenuItem
              data-testid="sidebar-continuous-diff"
              active={isRouteActive('/comparison-diff')}
              icon={<Icon icon={faChartBar} />}
            >
              Diff View
              <NavLink to={{ pathname: '/comparison-diff', search }} exact />
            </MenuItem>
          </SubMenu>
          {isExperimentalAdhocUIEnabled && adhoc}
        </Menu>
      </SidebarContent>
      <SidebarFooter>
        <Menu iconShape="square">
          <MenuItem icon={<Icon icon={faFileAlt} />}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://pyroscope.io/docs"
            >
              Documentation
            </a>
          </MenuItem>
          <MenuItem icon={<Icon icon={faSlack} />}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://pyroscope.io/slack"
            >
              Slack
            </a>
          </MenuItem>
          <MenuItem icon={<Icon icon={faGithub} />}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://github.com/pyroscope-io/pyroscope"
            >
              Github
            </a>
          </MenuItem>
          {(window as any).isAuthRequired && (
            <MenuItem
              onClick={() => signOut()}
              icon={<Icon icon={faSignOutAlt} />}
            >
              Sign out
            </MenuItem>
          )}
          <MenuItem
            className={`${styles.collapseIcon} ${
              collapsed ? styles.collapsedIconCollapsed : ''
            }`}
            onClick={toggleCollapse}
            icon={<Icon icon={faChevronLeft} />}
          >
            Collapse Sidebar
          </MenuItem>
        </Menu>
      </SidebarFooter>
    </Sidebar>
  );
}
