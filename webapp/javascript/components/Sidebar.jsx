import React, { useState, useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { withShortcut } from 'react-keybind';
import Modal from 'react-modal';
import clsx from 'clsx';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faKeyboard } from '@fortawesome/free-solid-svg-icons/faKeyboard';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faBell } from '@fortawesome/free-solid-svg-icons/faBell';
import { faSignOutAlt } from '@fortawesome/free-solid-svg-icons/faSignOutAlt';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import { useLocation, NavLink } from 'react-router-dom';
import ShortcutsModal from './ShortcutsModal';
import SlackIcon from './SlackIcon';

import { fetchNames } from '../redux/actions';
import history from '../util/history';

const modalStyle = {
  overlay: {
    backgroundColor: 'rgba(0,0,0,0.75)',
  },
  content: {
    background: '#222',
    border: '1px solid #111',
  },
};

function SidebarItem(props) {
  const { children, tooltipText } = props;
  return (
    <div className="sidebar-item">
      {children}
      <div className="sidebar-tooltip-wrapper">
        {tooltipText && <span className="sidebar-tooltip">{tooltipText}</span>}
      </div>
    </div>
  );
}

function signOut() {
  const form = document.createElement('form');

  form.method = 'POST';
  form.action = '/logout';

  document.body.appendChild(form);

  form.submit();
}

function Sidebar(props) {
  const { shortcut } = props;

  const [shortcutsModalOpen, setShortcutsModalOpen] =
    useState(shortcutsModalOpen);

  const { search } = useLocation();

  const showShortcutsModal = () => {
    setShortcutsModalOpen(true);
  };

  const closeShortcutsModal = () => {
    setShortcutsModalOpen(false);
  };

  useEffect(() => {
    shortcut.registerShortcut(
      showShortcutsModal,
      ['shift+?'],
      'Shortcuts',
      'Show Keyboard Shortcuts Modal'
    );
  }, []);

  return (
    <div className="sidebar">
      <span className="logo" onClick={() => history.push('/')} />
      <SidebarItem tooltipText="Single View">
        <NavLink
          activeClassName="active-route"
          data-testid="sidebar-root"
          to={{ pathname: '/', search }}
          exact
        >
          <FontAwesomeIcon icon={faWindowMaximize} />
        </NavLink>
      </SidebarItem>
      <SidebarItem tooltipText="Comparison View">
        <NavLink
          activeClassName="active-route"
          data-testid="sidebar-comparison"
          to={{ pathname: '/comparison', search }}
          exact
        >
          <FontAwesomeIcon icon={faColumns} />
        </NavLink>
      </SidebarItem>
      <SidebarItem tooltipText="Diff View">
        <NavLink
          activeClassName="active-route"
          data-testid="sidebar-comparison-diff"
          to={{ pathname: '/comparison-diff', search }}
          exact
        >
          <FontAwesomeIcon icon={faChartBar} />
        </NavLink>
      </SidebarItem>
      <SidebarItem tooltipText="Adhoc">
        <NavLink
          activeClassName="active-route"
          data-testid="achoc-single"
          to={{ pathname: '/adhoc-single', search }}
          exact
        >
          <FontAwesomeIcon icon={faWindowMaximize} />
        </NavLink>
      </SidebarItem>
      <SidebarItem tooltipText="Alerts - Coming Soon">
        <button type="button">
          <FontAwesomeIcon icon={faBell} />
        </button>
      </SidebarItem>
      <div className="sidebar-space-filler" />
      <SidebarItem tooltipText="Docs" externalLink>
        <a rel="noreferrer" target="_blank" href="https://pyroscope.io/docs">
          <FontAwesomeIcon icon={faFileAlt} />
        </a>
      </SidebarItem>
      <SidebarItem tooltipText="Slack" externalLink>
        <a rel="noreferrer" target="_blank" href="https://pyroscope.io/slack">
          <SlackIcon />
        </a>
      </SidebarItem>
      <SidebarItem tooltipText="GitHub" externalLink>
        <a
          rel="noreferrer"
          target="_blank"
          href="https://github.com/pyroscope-io/pyroscope"
        >
          <FontAwesomeIcon icon={faGithub} />
        </a>
      </SidebarItem>
      <SidebarItem tooltipText="Keyboard Shortcuts">
        <button
          onClick={showShortcutsModal}
          type="button"
          id="tests-shortcuts-btn"
        >
          <FontAwesomeIcon icon={faKeyboard} />
        </button>
      </SidebarItem>
      {window.isAuthRequired ? (
        <SidebarItem tooltipText="Sign Out">
          <button type="button" onClick={() => signOut()}>
            <FontAwesomeIcon icon={faSignOutAlt} />
          </button>
        </SidebarItem>
      ) : (
        []
      )}
      <Modal
        isOpen={shortcutsModalOpen}
        style={modalStyle}
        appElement={document.getElementById('root')}
        ariaHideApp={false}
      >
        <div className="modal-close-btn" onClick={closeShortcutsModal} />
        <ShortcutsModal closeModal={closeShortcutsModal} />
      </Modal>
    </div>
  );
}

export default connect((x) => x, { fetchNames })(withShortcut(Sidebar));
