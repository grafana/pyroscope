import React from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons/faWindowMaximize';
import { faInfoCircle } from '@fortawesome/free-solid-svg-icons/faInfoCircle';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';

import SidebarUI, {
  MenuItem,
  SidebarHeader,
  SidebarFooter,
  SidebarContent,
  Menu,
} from '@pyroscope/ui/Sidebar';
import { useLocation, NavLink } from 'react-router-dom';
import Icon from '@pyroscope/ui/Icon';
import clsx from 'clsx';
import { isRouteActive, ROUTES } from '@pyroscope/pages/routes';
import Logo from '@pyroscope/static/logo.svg';
import styles from './Sidebar.module.css';
import { SidebarTenant } from '@pyroscope/components/SidebarTenant';

export function Sidebar() {
  const collapsed = true;

  const { search, pathname } = useLocation();

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
          <SidebarTenant />
        </Menu>
      </SidebarContent>
      <SidebarFooter>
        <Menu iconShape="square">
          <MenuItem icon={<Icon icon={faInfoCircle} />}>
            <a href="/admin">Admin Page</a>
          </MenuItem>
          <MenuItem icon={<Icon icon={faFileAlt} />}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://grafana.com/docs/pyroscope"
            >
              Documentation
            </a>
          </MenuItem>
          <MenuItem icon={<Icon icon={faGithub} />}>
            <a
              rel="noreferrer"
              target="_blank"
              href="https://github.com/grafana/pyroscope"
            >
              Github
            </a>
          </MenuItem>
        </Menu>
      </SidebarFooter>
    </SidebarUI>
  );
}
