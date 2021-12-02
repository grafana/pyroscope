import React, { useEffect } from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faSlack } from '@fortawesome/free-brands-svg-icons/faSlack';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import { faKeyboard } from '@fortawesome/free-solid-svg-icons/faKeyboard';
import Sidebar, {
  MenuItem,
  SidebarFooter,
  SidebarContent,
  SubMenu,
  Menu,
} from '@ui/Sidebar';
import { useLocation, NavLink } from 'react-router-dom';
import { isExperimentalAdhocUIEnabled } from '@utils/features';
import styles from './Sidebar.module.css';

export default function Sidebar2() {
  const { search, pathname } = useLocation();

  // the component doesn't seem to support setting up an active item
  // so we must set it up manually
  // https://github.com/azouaoui-med/react-pro-sidebar/issues/84
  const isRouteActive = function (route: string) {
    return pathname === route;
  };

  // notice how there's no SubMenu here
  // since that's only rendered when Adhoc is enabled
  const continuousOnly = (
    <>
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
      <MenuItem active={isRouteActive('/comparison-diff')} icon={faChartBar}>
        Diff View
        <NavLink to={{ pathname: '/comparison-diff', search }} exact />
      </MenuItem>
    </>
  );

  const continuousAndAdhoc = (
    <>
      <SubMenu title="Continuous Profiling">{continuousOnly}</SubMenu>
      <SubMenu title="Adhoc Profiling">
        <MenuItem
          active={isRouteActive('/adhoc-single')}
          icon={faWindowMaximize}
        >
          Single View
        </MenuItem>
        <MenuItem active={isRouteActive('/adhoc-comparison')} icon={faColumns}>
          Comparison View
        </MenuItem>
        <MenuItem active={isRouteActive('/adhoc-diff')} icon={faChartBar}>
          Diff View
        </MenuItem>
      </SubMenu>
    </>
  );

  return (
    <Sidebar>
      <SidebarContent>
        <Menu iconShape="square">
          {isExperimentalAdhocUIEnabled ? continuousAndAdhoc : continuousOnly}
        </Menu>
      </SidebarContent>
      <SidebarFooter>
        <Menu iconShape="square">
          <MenuItem icon={faFileAlt}>Documentation</MenuItem>
          <MenuItem icon={faSlack}>Slack</MenuItem>
          <MenuItem icon={faGithub}>Github</MenuItem>
          <MenuItem icon={faKeyboard}>Shortcuts</MenuItem>
        </Menu>
      </SidebarFooter>
    </Sidebar>
  );
}
