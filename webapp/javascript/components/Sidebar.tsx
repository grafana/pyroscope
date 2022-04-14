import React, { useMemo } from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faCog } from '@fortawesome/free-solid-svg-icons/faCog';
import { faInfoCircle } from '@fortawesome/free-solid-svg-icons/faInfoCircle';
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
} from '@webapp/ui/Sidebar';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  selectSidebarCollapsed,
  collapseSidebar,
  uncollapseSidebar,
  recalculateSidebar,
} from '@webapp/redux/reducers/ui';
import { useLocation, NavLink } from 'react-router-dom';
import { isAdhocUIEnabled } from '@webapp/util/features';
import Icon from '@webapp/ui/Icon';
import { useWindowWidth } from '@react-hook/window-size';
import styles from './Sidebar.module.css';
import { PAGES } from '../pages/constants';

function signOut() {
  // By visiting /logout we're clearing jwtCookie
  fetch('/logout').then((d) => {
    (window as Window).location = PAGES.LOGIN;
  });
}

export function SidebarComponent() {
  const collapsed = useAppSelector(selectSidebarCollapsed);
  const dispatch = useAppDispatch();

  const { search, pathname } = useLocation();
  const windowWidth = useWindowWidth();
  // @todo: use `features` here
  const authEnabled = (window as ShamefulAny).isAuthRequired;

  // the component doesn't seem to support setting up an active item
  // so we must set it up manually
  // https://github.com/azouaoui-med/react-pro-sidebar/issues/84
  const isRouteActive = (route: string) => {
    if (
      route === PAGES.CONTINOUS_SINGLE_VIEW ||
      route === PAGES.COMPARISON_VIEW ||
      route === PAGES.ADHOC_COMPARISON
    ) {
      return pathname === route;
    }

    return pathname.startsWith(route);
  };

  const isSidebarVisible = useMemo(
    () =>
      (
        [
          PAGES.CONTINOUS_SINGLE_VIEW,
          PAGES.COMPARISON_VIEW,
          PAGES.ADHOC_COMPARISON,
          PAGES.COMPARISON_DIFF_VIEW,
          PAGES.SETTINGS,
          PAGES.SERVICE_DISCOVERY,
          PAGES.ADHOC_SINGLE,
          PAGES.ADHOC_COMPARISON,
          PAGES.ADHOC_COMPARISON_DIFF,
        ] as string[]
      ).includes(pathname) || pathname.startsWith(PAGES.SETTINGS),
    [pathname]
  );

  React.useLayoutEffect(() => {
    dispatch(recalculateSidebar());
  }, [windowWidth]);

  // TODO
  // simplify this
  const isContinuousActive =
    isRouteActive(PAGES.CONTINOUS_SINGLE_VIEW) ||
    isRouteActive(PAGES.COMPARISON_VIEW) ||
    isRouteActive(PAGES.COMPARISON_DIFF_VIEW);
  const isAdhocActive =
    isRouteActive(PAGES.ADHOC_SINGLE) ||
    isRouteActive(PAGES.ADHOC_COMPARISON) ||
    isRouteActive(PAGES.ADHOC_COMPARISON_DIFF);
  const isSettingsActive = isRouteActive(PAGES.SETTINGS);

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
        active={isRouteActive(PAGES.ADHOC_SINGLE)}
        icon={<Icon icon={faWindowMaximize} />}
      >
        Single View
        <NavLink to={{ pathname: PAGES.ADHOC_SINGLE, search }} exact />
      </MenuItem>
      <MenuItem
        data-testid="sidebar-adhoc-comparison"
        active={isRouteActive(PAGES.ADHOC_COMPARISON)}
        icon={<Icon icon={faColumns} />}
      >
        Comparison View
        <NavLink to={{ pathname: PAGES.ADHOC_COMPARISON, search }} exact />
      </MenuItem>
      <MenuItem
        data-testid="sidebar-adhoc-comparison-diff"
        active={isRouteActive(PAGES.ADHOC_COMPARISON_DIFF)}
        icon={<Icon icon={faChartBar} />}
      >
        Diff View
        <NavLink to={{ pathname: PAGES.ADHOC_COMPARISON_DIFF, search }} exact />
      </MenuItem>
    </SubMenu>
  );

  const toggleCollapse = () => {
    const action = collapsed ? uncollapseSidebar : collapseSidebar;
    dispatch(action());
  };

  return isSidebarVisible ? (
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
              active={isRouteActive(PAGES.CONTINOUS_SINGLE_VIEW)}
              icon={<Icon icon={faWindowMaximize} />}
            >
              Single View
              <NavLink
                activeClassName="active-route"
                data-testid="sidebar-root"
                to={{ pathname: PAGES.CONTINOUS_SINGLE_VIEW, search }}
                exact
              />
            </MenuItem>
            <MenuItem
              data-testid="sidebar-continuous-comparison"
              active={isRouteActive(PAGES.COMPARISON_VIEW)}
              icon={<Icon icon={faColumns} />}
            >
              Comparison View
              <NavLink to={{ pathname: PAGES.COMPARISON_VIEW, search }} exact />
            </MenuItem>
            <MenuItem
              data-testid="sidebar-continuous-diff"
              active={isRouteActive(PAGES.COMPARISON_DIFF_VIEW)}
              icon={<Icon icon={faChartBar} />}
            >
              Diff View
              <NavLink
                to={{ pathname: PAGES.COMPARISON_DIFF_VIEW, search }}
                exact
              />
            </MenuItem>
          </SubMenu>
          {isAdhocUIEnabled && adhoc}
        </Menu>
      </SidebarContent>
      <SidebarFooter>
        <Menu iconShape="square">
          {authEnabled && (
            <MenuItem
              data-testid="sidebar-settings"
              active={isSettingsActive}
              icon={<Icon icon={faCog} />}
            >
              Settings
              <NavLink to={{ pathname: PAGES.SETTINGS, search }} exact />
            </MenuItem>
          )}
          <MenuItem icon={<Icon icon={faInfoCircle} />}>
            Scrape Targets
            <NavLink to={{ pathname: PAGES.SERVICE_DISCOVERY, search }} exact />
          </MenuItem>
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
          {(window as ShamefulAny).isAuthRequired && (
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
  ) : null;
}

export default SidebarComponent;
