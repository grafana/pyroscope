import React, { useEffect, useState } from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faSlack } from '@fortawesome/free-brands-svg-icons/faSlack';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import { faChevronLeft } from '@fortawesome/free-solid-svg-icons/faChevronLeft';
import { faChevronRight } from '@fortawesome/free-solid-svg-icons/faChevronRight';
import { faKeyboard } from '@fortawesome/free-solid-svg-icons/faKeyboard';
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
import { useLocation, NavLink } from 'react-router-dom';
import { isExperimentalAdhocUIEnabled } from '@utils/features';
import { useWindowWidth } from '@react-hook/window-size';
import styles from './Sidebar.module.css';
import Logo from '../../images/logo-v3-small.svg';

export default function Sidebar2() {
  const { search, pathname } = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const windowWidth = useWindowWidth();

  // the component doesn't seem to support setting up an active item
  // so we must set it up manually
  // https://github.com/azouaoui-med/react-pro-sidebar/issues/84
  const isRouteActive = function (route: string) {
    return pathname === route;
  };

  useEffect(() => {
    const c = windowWidth < 1200;
    setCollapsed(c);
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
    isRouteActive('/adhoc-diff');

  const adhoc = (
    <SubMenu
      title="Adhoc Profiling"
      icon={faHandPointRight}
      active={isAdhocActive}
      defaultOpen={isAdhocActive}
    >
      <MenuItem active={isRouteActive('/adhoc-single')} icon={faWindowMaximize}>
        Single View
      </MenuItem>
      <MenuItem active={isRouteActive('/adhoc-comparison')} icon={faColumns}>
        Comparison View
      </MenuItem>
      <MenuItem active={isRouteActive('/adhoc-diff')} icon={faChartBar}>
        Diff View
      </MenuItem>
    </SubMenu>
  );

  const toggleCollapse = () => setCollapsed(!collapsed);

  return (
    <Sidebar collapsed={collapsed}>
      <SidebarHeader>
        <div className={styles.logo}>
          <img src={Logo} alt="Logo" width={36} height={36} />
          <b className={`${collapsed ? styles.logoTextCollapsed : ''}`}>
            Pyroscope
          </b>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <Menu iconShape="square" popperArrow>
          <SubMenu
            title="Continuous Profiling"
            icon={faSync}
            active={isContinuousActive}
            defaultOpen={isContinuousActive}
          >
            <MenuItem active={isRouteActive('/')} icon={faWindowMaximize}>
              Single View
              <NavLink
                activeClassName="active-route"
                data-testid="sidebar-root"
                to={{ pathname: '/', search }}
                exact
              />
            </MenuItem>
            <MenuItem active={isRouteActive('/comparison')} icon={faColumns}>
              Comparison View
              <NavLink to={{ pathname: '/comparison', search }} exact />
            </MenuItem>
            <MenuItem
              active={isRouteActive('/comparison-diff')}
              icon={faChartBar}
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
          <MenuItem icon={faFileAlt}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://pyroscope.io/docs"
            >
              Documentation
            </a>
          </MenuItem>
          <MenuItem icon={faSlack}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://pyroscope.io/slack"
            >
              Slack
            </a>
          </MenuItem>
          <MenuItem icon={faGithub}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://github.com/pyroscope-io/pyroscope"
            >
              Github
            </a>
          </MenuItem>
          <MenuItem icon={faKeyboard}>Shortcuts</MenuItem>
          <MenuItem
            className={`${styles.collapseIcon} ${
              collapsed ? styles.collapsedIconCollapsed : ''
            }`}
            onClick={toggleCollapse}
            icon={faChevronLeft}
          >
            Collapse Sidebar
          </MenuItem>
        </Menu>
      </SidebarFooter>
    </Sidebar>
  );
}
